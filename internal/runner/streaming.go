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
// It does not implement recovery, checkpoint/resume or exactly-once delivery.

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
	// any transaction exists.
	if err := e.dryRun(ctx, req.Plan); err != nil {
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
		batch: req.BatchSize, sequence: 0,
	}
	summary = contracts.StreamingApplySummary{
		Protocol: req.Protocol, PlanID: req.Plan.ID, RunID: req.RunID,
		ReceiptPageSize: req.ReceiptPageSize, State: contracts.RunStateRunning,
		Audit: contracts.AuditStateConsistent,
	}
	run.summary = &summary
	for _, table := range req.Plan.Tables {
		if err := run.applyTable(ctx, table); err != nil {
			return run.finish(err)
		}
	}
	// 9. Independent read-only validation over the modified target. A failed
	// validation can never be reported as a complete run.
	report, verr := e.Validate(ctx, contracts.ValidateRequest{Plan: req.Plan, Strict: true})
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

func (r *streamingRun) finish(cause error) (contracts.StreamingApplySummary, error) {
	summary := *r.summary
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
	if e.streamingApprovals == nil || reflect.ValueOf(e.streamingApprovals).IsNil() {
		return contracts.ErrUnsafe
	}
	if err := contracts.ValidateStreamingApproval(req.Plan, req.Approval, time.Now()); err != nil {
		return err
	}
	digest, err := history.CanonicalPlanDigest(req.Plan)
	if err != nil {
		return err
	}
	record, err := e.streamingApprovals.VerifyStreaming(ctx, req.Plan, req.Approval)
	if err != nil {
		return approvalRefusal(err)
	}
	if record.PlanContentDigest != digest ||
		record.Protocol != contracts.StreamingProtocolRevision ||
		record.BindingRevision != contracts.ApprovalBindingRevision ||
		!reflect.DeepEqual(record.StreamingApproval(), req.Approval) {
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
	summary  *contracts.StreamingApplySummary
	batch    int
	sequence int64
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
		left, err := r.engine.source.Page(ctx, request)
		if err != nil {
			return err
		}
		right, err := r.engine.target.Page(ctx, request)
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
		changes, err := r.engine.pageChanges(ctx, table, left.Rows, right.Rows)
		if err != nil {
			return err
		}
		if err := r.writeBatch(ctx, table, changes); err != nil {
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
func (r *streamingRun) writeBatch(ctx context.Context, table contracts.TablePlan, changes []contracts.RowChange) error {
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
	return nil
}

// classify records the bounded batch counters for one terminal batch failure and
// returns the original error so the caller stops.
func (r *streamingRun) classify(outcome contracts.BatchOutcome, cause error) error {
	r.summary.BatchesTotal++
	switch outcome {
	case contracts.BatchCommitted:
		r.summary.BatchesCommitted++
	case contracts.BatchCommitUnknown, contracts.BatchAuditIncomplete:
		r.summary.BatchesUnknown++
		r.summary.Audit = contracts.AuditStateIncomplete
		r.summary.State = contracts.RunStateCommitUnknown
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
