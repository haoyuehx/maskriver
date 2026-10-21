package runner

// Streaming Apply (m2-stream-v1): the bounded, protocol-bound execution path.
//
// The legacy Apply path and its ApplyReport are untouched. This path is explicit
// (StreamingProtocolRevision plus ExplicitApply), requires a verified streaming
// approval bound to the canonical plan content, and writes each page through
// the WP2 SQLite outbox so the business UPDATEs and the receipt INSERT share one
// transaction. Receipts are never accumulated in memory: only constant-size
// counters are updated per batch, and receipts are read back through a bounded
// ReceiptReader.
//
// Recovery/checkpoint/resume is enabled only when the recovery schema exists;
// the fresh StreamingApply path then records a manifest and progress inside the
// same transactions. Exactly-once delivery is still not claimed.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/internal/testhooks"
	"github.com/haoyuehx/maskriver/internal/verify"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// StreamingApprovalQuery is the read-only verification seam for streaming
// approvals. history.Store implements it. A nil seam refuses the run before any
// database write, and it never falls back to a legacy approval.
type StreamingApprovalQuery interface {
	VerifyStreaming(context.Context, contracts.Plan, contracts.StreamingApproval) (history.StreamingRecord, error)
}

// SetStreamingApprovalQuery injects the read-only streaming approval verifier.
// A nil verifier is not permissive: StreamingApply refuses with ErrUnsafe.
func (e *Engine) SetStreamingApprovalQuery(query StreamingApprovalQuery) {
	if e != nil {
		e.streamingApprovals = query
	}
}

// StreamingApply runs the explicit bounded streaming path and returns a
// constant-size summary. Every pre-write gate (structure, approval, live plan
// preflight, full dry run) completes before the outbox is opened or a run row is
// created.
func (e *Engine) StreamingApply(ctx context.Context, req contracts.StreamingApplyRequest) (contracts.StreamingApplySummary, error) {
	var summary contracts.StreamingApplySummary
	if e == nil || ctx == nil {
		return summary, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	// 1. Structural and budget validation. The capability declaration is
	// validated separately, after the real outbox is opened and probed.
	if err := validateStreamingStructure(req, e.cfg); err != nil {
		return summary, fmt.Errorf("validateStreamingStructure: %w", err)
	}
	// 2. Reserved outbox tables are never business scopes, on this or any other
	// path. The CLI and config are checked again here because a caller may
	// construct the request directly.
	if err := e.checkReservedPlan(req.Plan); err != nil {
		return summary, fmt.Errorf("checkReservedPlan: %w", err)
	}
	// 3. Authenticate the streaming approval against the history store. A
	// missing query, a legacy record, a forged signature, an unknown signer, a
	// revoked or expired record, or a plan-content mismatch all refuse here.
	if err := e.checkStreamingApproval(ctx, req); err != nil {
		return summary, fmt.Errorf("checkStreamingApproval: %w", err)
	}
	// 4. Re-derive the plan from the live snapshots and run the whole-plan
	// static capability gate. This is the same gate the legacy Apply uses.
	if err := e.preflight(ctx, req.Plan); err != nil {
		return summary, fmt.Errorf("preflight: %w", err)
	}
	// 5. Full read-only dry run: every planned row and column is compared before
	// any transaction exists. The streaming dry run uses the same row-byte
	// budget as the apply loop, so it can not materialise an oversized field and
	// then claim the run was bounded.
	if err := e.dryRunWithin(ctx, req.Plan, req.Budget.MaxRowBytes); err != nil {
		return summary, fmt.Errorf("dryRun: %w", err)
	}
	// 6. Open the explicit outbox only after all pre-write gates pass. Opening
	// verifies platform, path identity, journal mode, session PRAGMAs and the
	// pre-provisioned schema; no DDL runs here.
	opts, err := e.streamingOutboxOptions(req)
	if err != nil {
		return summary, err
	}
	outbox, err := db.OpenOutbox(ctx, opts)
	if err != nil {
		return summary, fmt.Errorf("openOutbox: %w", err)
	}
	defer outbox.Close()
	caps := outbox.Capabilities()
	if err := caps.Validate(req.Protocol); err != nil {
		return summary, err
	}
	// A caller-declared capability set must not be more permissive than the real
	// store. The canonical form is the one probed from the open outbox.
	if req.Outbox != (contracts.OutboxCapabilities{}) && req.Outbox != caps {
		return summary, contracts.ErrUnsupported
	}
	req.Outbox = caps
	if req.ReceiptPageSize > caps.MaxReceiptPage {
		return summary, contracts.ErrInvalid
	}
	if err := req.Validate(time.Now()); err != nil {
		return summary, err
	}
	// 7. Persist the run identity. This is a separate autocommit operation and
	// is never reported as a committed business batch.
	contentDigest, err := history.CanonicalPlanDigest(req.Plan)
	if err != nil {
		return summary, err
	}
	if err := outbox.InitializeRun(ctx, db.OutboxRun{
		RunID: req.RunID, PlanID: req.Plan.ID,
		ApprovalBinding: contracts.ApprovalBindingRevision,
		TargetDigest:    req.Plan.TargetID, ScopeDigest: contentDigest,
		Protocol: req.Protocol,
	}); err != nil {
		return summary, fmt.Errorf("initializeRun: %w", err)
	}
	// 8. Execute table by table with a stable keyset cursor and a stable batch
	// sequence across the whole run.
	run := &streamingRun{
		engine: e, outbox: outbox, summary: &summary,
		batch: req.BatchSize, sequence: 0, budget: req.Budget,
	}
	summary = contracts.StreamingApplySummary{
		Protocol: req.Protocol, PlanID: req.Plan.ID, RunID: req.RunID,
		ReceiptPageSize: req.ReceiptPageSize, State: contracts.RunStateRunning,
		Audit: contracts.AuditStateConsistent,
	}
	run.summary = &summary
	// A run is resumable only when the recovery schema was explicitly installed
	// before it started. Without it this remains exactly the Issue #13 path and
	// the run is never presented as recoverable.
	var manifest db.RecoveryManifest
	if available, aerr := outbox.RecoveryAvailable(ctx); aerr != nil {
		return summary, aerr
	} else if available {
		if err := validateRecoverablePlan(req.Plan); err != nil {
			return summary, err
		}
		recovery, rerr := outbox.OpenRecovery(ctx)
		if rerr != nil {
			return summary, rerr
		}
		identity, ierr := db.TargetIdentity(e.cfg.Target)
		if ierr != nil {
			return summary, contracts.ErrUnsupported
		}
		settings, serr := recoverySettingsDigest(req.Budget, req.BatchSize)
		if serr != nil {
			return summary, serr
		}
		manifest, rerr = recoveryManifestFor(req, identity, settings, e.key, time.Now())
		if rerr != nil {
			return summary, rerr
		}
		if rerr = recovery.CreateManifest(ctx, manifest); rerr != nil {
			return summary, rerr
		}
		run.recovery = recovery
		run.progress = db.RecoveryProgress{RunID: req.RunID}
	}
	if run.recovery != nil {
		if err := run.continueRecovery(ctx, req.Plan); err != nil {
			return run.finish(err)
		}
		// Business complete: strict validation, then the terminal state. A
		// failed validation is recorded truthfully and never reported complete.
		rreq := StreamingRecoveryRequest{
			Protocol: req.Protocol, Plan: req.Plan, Approval: req.Approval,
			ExplicitApply: req.ExplicitApply, RunID: req.RunID,
			BatchSize: req.BatchSize, ReceiptPageSize: req.ReceiptPageSize, Budget: req.Budget,
		}
		if err := testhooks.Fire("before_validate"); err != nil {
			return run.finish(err)
		}
		summary, verr := e.recoveryValidate(ctx, rreq, manifest, run.progress)
		if verr != nil {
			return summary, verr
		}
		if err := testhooks.Fire("after_validate_before_complete"); err != nil {
			return run.finish(err)
		}
		if err := run.recovery.SetManifestState(ctx, req.RunID, run.progress.Sequence,
			db.RecoveryStateComplete, true, db.ValidationPassed); err != nil {
			return run.finish(err)
		}
		summary.State = contracts.RunStateComplete
		summary.Audit = contracts.AuditStateConsistent
		summary.BatchesTotal = summary.BatchesCommitted
		return summary, nil
	}
	for _, table := range req.Plan.Tables {
		if err := run.applyTable(ctx, table); err != nil {
			return run.finish(err)
		}
	}
	// 9. Independent read-only validation over the modified target. A failed
	// validation can never be reported as a complete run.
	report, verr := e.validateWithin(ctx, contracts.ValidateRequest{Plan: req.Plan, Strict: true}, req.Budget.MaxRowBytes)
	var overflow bool
	summary.IssueCounts, overflow = boundedIssues(report.Issues, req.Budget.MaxIssueCounts)
	if overflow {
		summary.State = contracts.RunStateIncomplete
		summary.Audit = contracts.AuditStateIncomplete
		return run.finish(contracts.ErrIncomplete)
	}
	if verr != nil {
		summary.State = contracts.RunStateIncomplete
		summary.Audit = contracts.AuditStateIncomplete
		return run.finish(verr)
	}
	if !report.Passed(true) {
		summary.State = contracts.RunStateIncomplete
		summary.Audit = contracts.AuditStateIncomplete
		return run.finish(contracts.ErrIncomplete)
	}
	summary.BatchesTotal = summary.BatchesCommitted
	summary.State = contracts.RunStateComplete
	summary.Audit = contracts.AuditStateConsistent
	if err := summary.Validate(req.Budget); err != nil {
		return summary, fmt.Errorf("summaryValidate: %w", err)
	}
	return summary, nil
}

// validateWithin runs the independent validation through the byte-aware reader,
// so the post-apply check is bounded by the same MaxRowBytes as the run. Legacy
// Validate is unchanged.
func (e *Engine) validateWithin(ctx context.Context, req contracts.ValidateRequest, maxRowBytes int64) (contracts.ValidationReport, error) {
	if err := req.Plan.ValidateShape(); err != nil {
		return contracts.ValidationReport{}, err
	}
	if err := e.checkPlan(ctx, req.Plan); err != nil {
		return contracts.ValidationReport{}, err
	}
	source := boundedValidationReader{Reader: e.source, maxRowBytes: maxRowBytes}
	target := boundedValidationReader{Reader: e.target, maxRowBytes: maxRowBytes}
	return verify.New().Validate(ctx, source, target, req)
}

// boundedValidationReader overrides only Page; every other reader method is the
// underlying adapter's. Validation therefore keeps its frozen contract while a
// page that cannot be proven within the row budget is refused.
type boundedValidationReader struct {
	contracts.Reader
	maxRowBytes int64
}

func (r boundedValidationReader) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	bounded, ok := r.Reader.(db.BoundedReader)
	if !ok {
		return contracts.Page{}, contracts.ErrUnsupported
	}
	return bounded.PageWithin(ctx, req, r.maxRowBytes)
}

func (r *streamingRun) finish(cause error) (contracts.StreamingApplySummary, error) {
	summary := *r.summary
	if r.recovery != nil {
		state, validation := recoveryFailureState(cause)
		if !errors.Is(cause, contracts.ErrCommitUnknown) {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = r.recovery.SetManifestState(cleanup, summary.RunID, r.progress.Sequence, state,
				r.progress.BusinessComplete, validation)
			cancel()
		}
	}
	if summary.State == contracts.RunStateRunning {
		switch {
		case summary.BatchesUnknown > 0:
			summary.State = contracts.RunStateCommitUnknown
			summary.Audit = contracts.AuditStateIncomplete
		default:
			summary.State = contracts.RunStateIncomplete
		}
	}
	if cause == nil {
		cause = contracts.ErrIncomplete
	}
	return summary, cause
}

// validateStreamingStructure checks only what can be judged before any I/O. The
// capability declaration is validated later, against the probed outbox.
func validateStreamingStructure(req contracts.StreamingApplyRequest, cfg config.Runtime) error {
	if req.Protocol != contracts.StreamingProtocolRevision {
		return contracts.ErrUnsupported
	}
	if !req.ExplicitApply {
		return contracts.ErrUnsafe
	}
	if !contracts.ValidRunID(req.RunID) {
		return contracts.ErrInvalid
	}
	if err := req.Budget.Validate(); err != nil {
		return err
	}
	if err := contracts.ValidatePlanIdentity(req.Plan); err != nil {
		return err
	}
	if err := req.Budget.ValidatePlan(req.Plan); err != nil {
		return err
	}
	if req.BatchSize <= 0 || req.BatchSize > req.Budget.MaxBatchSize ||
		req.BatchSize > 1000 {
		return contracts.ErrInvalid
	}
	if req.ReceiptPageSize <= 0 || req.ReceiptPageSize > req.Budget.MaxReceiptPage {
		return contracts.ErrInvalid
	}
	if cfg.Workers < 1 || cfg.Workers > req.Budget.MaxWorkers {
		return contracts.ErrInvalid
	}
	// MaxQueue is the internal sink queue. This implementation only supports
	// synchronous delivery, so a non-zero queue is an unsupported request rather
	// than a silently ignored setting.
	if req.Budget.MaxQueue != 0 {
		return contracts.ErrUnsupported
	}
	// Reject a worst case that cannot fit MaxProcessBytes before any I/O.
	if err := validateProcessBudgetCombination(req.Budget); err != nil {
		return err
	}
	return nil
}

// checkReservedPlan refuses a plan that names a reserved outbox table, at any
// case-insensitive spelling SQLite could resolve to the same table.
func (e *Engine) checkReservedPlan(plan contracts.Plan) error {
	for _, table := range plan.Tables {
		if reservedOutboxName(table.Table.Table) {
			return contracts.ErrUnsafe
		}
	}
	for _, scope := range e.cfg.ResolvedScopes() {
		if reservedOutboxName(scope.Table) {
			return contracts.ErrUnsafe
		}
	}
	return nil
}

func reservedOutboxName(name string) bool {
	return db.IsOutboxTable(name) || db.IsOutboxTable(lowerASCII(name))
}

func lowerASCII(value string) string {
	out := []byte(value)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}

// checkStreamingApproval requires a verifier that authenticates the stored
// streaming record. The frozen contracts.ValidateStreamingApproval only checks
// structure; the store remains authoritative for signature, trust, expiry and
// revocation, and the canonical content digest covers the per-column fields
// Plan.ID does not.
func (e *Engine) checkStreamingApproval(ctx context.Context, req contracts.StreamingApplyRequest) error {
	return e.checkStreamingApprovalPlan(ctx, req.Plan, req.Approval)
}

// checkStreamingApprovalPlan is the plan/approval form shared by StreamingApply
// and StreamingResume. It never falls back to a legacy record.
func (e *Engine) checkStreamingApprovalPlan(ctx context.Context, plan contracts.Plan, approval contracts.StreamingApproval) error {
	if e.streamingApprovals == nil || reflect.ValueOf(e.streamingApprovals).IsNil() {
		return contracts.ErrUnsafe
	}
	if err := contracts.ValidateStreamingApproval(plan, approval, time.Now()); err != nil {
		return err
	}
	digest, err := history.CanonicalPlanDigest(plan)
	if err != nil {
		return err
	}
	record, err := e.streamingApprovals.VerifyStreaming(ctx, plan, approval)
	if err != nil {
		return approvalRefusal(err)
	}
	if record.PlanContentDigest != digest ||
		record.Protocol != contracts.StreamingProtocolRevision ||
		record.BindingRevision != contracts.ApprovalBindingRevision ||
		!reflect.DeepEqual(record.StreamingApproval(), approval) {
		return contracts.ErrUnsafe
	}
	return nil
}

func (e *Engine) streamingOutboxOptions(req contracts.StreamingApplyRequest) (db.OutboxOptions, error) {
	opts, err := e.cfg.DatabaseOptions(false)
	if err != nil {
		return db.OutboxOptions{}, err
	}
	// The pinned writer needs one further pooled connection for paged reads.
	opts.MaxOpenConns = 2
	return db.OutboxOptions{
		DatabaseOptions: opts,
		Protocol:        req.Protocol,
		JournalMode:     "delete",
		MaxReceiptPage:  req.Budget.MaxReceiptPage,
	}, nil
}

// streamingRun carries the constant-size state of one streaming execution.
type streamingRun struct {
	engine   *Engine
	outbox   *db.Outbox
	recovery *db.Recovery
	summary  *contracts.StreamingApplySummary
	progress db.RecoveryProgress
	batch    int
	sequence int64
	budget   contracts.ResourceBudget
}

// applyTable streams one planned table in batch-sized pages. Rows whose planned
// columns are all NULL are dropped (no statement, no receipt), exactly like the
// legacy path. A page that writes nothing consumes no sequence number.
func (r *streamingRun) applyTable(ctx context.Context, table contracts.TablePlan) error {
	rows, err := r.countTable(ctx, table)
	if err != nil {
		return err
	}
	cols := planColumns(table)
	var after []contracts.Value
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		request := contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: r.batch,
		}
		left, err := pageWithin(ctx, r.engine.source, request, r.budget.MaxRowBytes)
		if err != nil {
			return err
		}
		right, err := pageWithin(ctx, r.engine.target, request, r.budget.MaxRowBytes)
		if err != nil {
			return err
		}
		if len(left.Rows) != len(right.Rows) || left.Done != right.Done || len(left.Rows) > r.batch {
			return contracts.ErrConflict
		}
		if len(left.Rows) == 0 {
			if left.Done {
				break
			}
			return contracts.ErrIncomplete
		}
		// The byte-aware reader already rejected an oversized row before
		// materialising it; this second check validates the real decoded payload
		// (the SQL preflight uses an upper bound for numeric columns) and then
		// applies the process and disk admission rules before masking, SQL or
		// receipts exist.
		pageBytes, err := pagePayloadBytes(left.Rows, r.budget.MaxRowBytes)
		if err != nil {
			return err
		}
		if err := r.engine.checkPageResources(ctx, left.Rows, pageBytes, r.budget); err != nil {
			return err
		}
		changes, err := r.engine.pageChanges(ctx, table, left.Rows, right.Rows)
		if err != nil {
			return err
		}
		for _, change := range changes {
			if changePayloadBytes(change) > r.budget.MaxRowBytes {
				return contracts.ErrUnsafe
			}
		}
		if err := r.writeBatch(ctx, table, changes, pageBytes); err != nil {
			return err
		}
		if left.Done {
			break
		}
		if len(left.Next) != len(table.PrimaryKey) || len(right.Next) != len(table.PrimaryKey) {
			return contracts.ErrIncomplete
		}
		for i := range left.Next {
			if left.Next[i] != right.Next[i] {
				return contracts.ErrConflict
			}
		}
		after = append([]contracts.Value(nil), left.Next...)
	}
	end, err := r.countTable(ctx, table)
	if err != nil {
		return err
	}
	if end != rows {
		return contracts.ErrConflict
	}
	return nil
}

func (r *streamingRun) countTable(ctx context.Context, table contracts.TablePlan) (int64, error) {
	source, err := r.engine.source.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	target, err := r.engine.target.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	if source != target || source <= 0 {
		return 0, contracts.ErrConflict
	}
	return source, nil
}

// writeBatch commits one page through the outbox. The receipt is inserted by the
// same transaction and the commit is attempted once. An unknown outcome stops
// the run; it is never retried and never reported as success.
func (r *streamingRun) writeBatch(ctx context.Context, table contracts.TablePlan, changes []contracts.RowChange, pageBytes int64) error {
	pending := make([]contracts.RowChange, 0, len(changes))
	for _, change := range changes {
		if assignsNothing(change) {
			continue
		}
		pending = append(pending, change)
	}
	if len(pending) == 0 {
		return nil
	}
	// The outbox reader starts its cursor at 0, so batch sequences start at 0
	// and grow deterministically across the whole run.
	// Narrow the pre-write window as far as this layer can: re-check disk
	// admission immediately before the transaction starts.
	if err := checkDiskAdmission(r.engine.cfg.Target, pageBytes, r.budget.MaxDiskBytes); err != nil {
		return err
	}
	sequence := r.sequence
	receipt := contracts.StreamingBatchReceipt{
		Table: table.Table, BatchID: batchID(r.summary.RunID, sequence),
		Ordinal: sequence, Rows: int64(len(pending)), Outcome: contracts.BatchCommitted,
	}
	tx, err := r.outbox.BeginBatch(ctx, r.summary.RunID, table, receipt)
	if err != nil {
		if errors.Is(err, contracts.ErrUnsafe) || errors.Is(err, contracts.ErrInvalid) ||
			errors.Is(err, contracts.ErrNotFound) || errors.Is(err, contracts.ErrConflict) {
			return err
		}
		return r.classify(contracts.BatchFailedBeforeCommit, err)
	}
	attempted := false
	n, uerr := tx.Update(ctx, pending)
	if uerr != nil {
		_ = rollbackBounded(tx)
		return r.classify(contracts.ClassifyBatch(false, nil, nil), uerr)
	}
	if n != int64(len(pending)) {
		_ = rollbackBounded(tx)
		return r.classify(contracts.BatchFailedBeforeCommit, contracts.ErrConflict)
	}
	attempted = true
	cerr := tx.Commit(ctx)
	outcome := contracts.ClassifyBatch(attempted, cerr, nil)
	if cerr != nil {
		return r.classify(outcome, cerr)
	}
	r.sequence = sequence + 1
	r.summary.BatchesTotal++
	r.summary.BatchesCommitted++
	r.summary.RowsCommitted += n
	// The batch is already committed. If it pushed the target over the declared
	// disk budget, report a failure and stop; the receipt is not deleted.
	return checkDiskAfterCommit(r.engine.cfg.Target, r.budget.MaxDiskBytes)
}

// classify records the bounded batch counters for one terminal batch failure and
// returns the original error so the caller stops.
func (r *streamingRun) classify(outcome contracts.BatchOutcome, cause error) error {
	r.summary.BatchesTotal++
	switch outcome {
	case contracts.BatchCommitted:
		r.summary.BatchesCommitted++
	case contracts.BatchCommitUnknown:
		// Only a genuinely uncertain commit is reported as unknown.
		r.summary.BatchesUnknown++
		r.summary.Audit = contracts.AuditStateIncomplete
		r.summary.State = contracts.RunStateCommitUnknown
	case contracts.BatchAuditIncomplete:
		// Data may be committed but the receipt state is not confirmed. This is
		// an audit gap, not an unknown commit; it must not be counted as one.
		r.summary.BatchesFailed++
		r.summary.Audit = contracts.AuditStateIncomplete
		r.summary.State = contracts.RunStateAuditIncomplete
	case contracts.BatchCommitFailed, contracts.BatchFailedBeforeCommit:
		r.summary.BatchesFailed++
	default:
		r.summary.BatchesFailed++
	}
	return cause
}

// boundedIssues aggregates verifier issues by (status, code), capped by the
// budget. It never retains a per-row or per-column list.
func boundedIssues(issues []contracts.Issue, max int) ([]contracts.IssueCount, bool) {
	if max <= 0 || len(issues) == 0 {
		return nil, max <= 0 && len(issues) > 0
	}
	type key struct {
		status contracts.Status
		code   contracts.IssueCode
	}
	index := make(map[key]int)
	var counts []contracts.IssueCount
	for _, issue := range issues {
		k := key{issue.Status, issue.Code}
		if i, ok := index[k]; ok {
			counts[i].Count += issue.Count
			continue
		}
		if len(counts) >= max {
			// The category cap is a hard bound. Do not silently drop a distinct
			// category: report the overflow so the run fails closed instead of
			// understating the verification result.
			return nil, true
		}
		index[k] = len(counts)
		counts = append(counts, contracts.IssueCount{
			Status: issue.Status, Code: issue.Code, Count: issue.Count,
		})
	}
	return counts, false
}

func batchID(runID string, sequence int64) string {
	return runID + "-" + strconv.FormatInt(sequence, 10)
}
