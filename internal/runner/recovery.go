package runner

// Idempotent streaming recovery for the local SQLite m2-recovery-v1 protocol.
//
// A recovery never replays a committed batch. It re-reads the manifest and the
// append-only progress log, cross-checks them against the same-transaction
// receipts, and verifies the target's committed prefix against deterministically
// recomputed replacements and its uncommitted suffix against the immutable
// source. Only then does it continue from the reconstructed keyset cursor.
//
// CommitUnknown is never retried from the error. The database is reopened and
// the authoritative progress is used: a committed transaction advanced the
// checkpoint and has a receipt; an uncommitted one did not. A target that shows
// masked rows beyond the checkpoint, or unmasked rows inside it, is a CONFLICT
// and is refused, never overwritten.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/internal/testhooks"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// StreamingRecoveryRequest is the explicit resume request. It reuses the frozen
// m1a-v1 Plan and the m2-stream-v1 approval; it never creates a new run.
type StreamingRecoveryRequest struct {
	Protocol        string
	Plan            contracts.Plan
	Approval        contracts.StreamingApproval
	ExplicitApply   bool
	RunID           string
	BatchSize       int
	ReceiptPageSize int
	Budget          contracts.ResourceBudget
}

// RecoveryReport is the constant-size, non-sensitive state of one resume or
// status observation. It contains no value, key, DSN or plaintext primary key.
type RecoveryReport struct {
	Protocol           string
	RunID              string
	PlanID             string
	State              db.RecoveryState
	Validation         db.ValidationState
	TableOrdinal       int
	Tables             int
	RowsProcessed      int64
	RowsCommitted      int64
	CommittedBatches   int64
	CheckpointSequence int64
	BusinessComplete   bool
	AuditComplete      bool
	ResumeEligible     bool
}

// recoveryBindingSet is the metadata identity a resume must match exactly.
type recoveryBindingSet struct {
	keyCheck          string
	planContentDigest string
	scopeDigest       string
	sourceSnapshot    string
	targetID          string
	schemaDigest      string
	configDigest      string
	strategyID        string
	strategyVersion   string
	normalization     string
	keyID             string
	settingsDigest    string
}

// recoverySettingsDigest binds a resume to the same budget and batch size as the
// original run. It contains no key or value.
func recoverySettingsDigest(budget contracts.ResourceBudget, batchSize int) (string, error) {
	wire := struct {
		Budget    contracts.ResourceBudget
		BatchSize int
	}{budget, batchSize}
	payload, err := json.Marshal(wire)
	if err != nil {
		return "", contracts.ErrInvalid
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// recoveryKeyCheck is a key-check value (KCV): HMAC(key, domain||KeyID). It binds
// the actual key material to the run without storing the key. The key is a
// high-entropy >=32-byte secret, so the offline-oracle risk is bounded and
// documented; a wrong key file with the correct KeyID is refused before any
// write, including on a vacuous (rows_processed=0) prefix.
func recoveryKeyCheck(key contracts.Value, keyID string) (string, error) {
	if key.Kind() != contracts.Bytes || len(key.Payload()) < 32 || keyID == "" {
		return "", contracts.ErrInvalid
	}
	mac := hmac.New(sha256.New, []byte(key.Payload()))
	mac.Write([]byte("maskriver-recovery-v1/key-check\x00"))
	mac.Write([]byte(keyID))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// recoveryScopeDigest reduces the ordered table/column scopes to one digest.
func recoveryScopeDigest(plan contracts.Plan) string {
	h := sha256.New()
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", table.Table.Database,
				table.Table.Schema, table.Table.Table, column.Column.Column)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// planRecoveryBinding extracts the single strategy/normalization tuple the
// manifest records. ValidateStreamingApproval already refuses a mixed tuple.
func planRecoveryBinding(plan contracts.Plan) (strategyID, strategyVersion, normalization, keyID string, err error) {
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if strategyID == "" {
				strategyID = column.Strategy.ID
				strategyVersion = column.Strategy.Version
				normalization = column.NormalizationVersion
				keyID = column.KeyID
				continue
			}
			if strategyID != column.Strategy.ID || strategyVersion != column.Strategy.Version ||
				normalization != column.NormalizationVersion || keyID != column.KeyID {
				return "", "", "", "", contracts.ErrUnsafe
			}
		}
	}
	if strategyID == "" || strategyVersion == "" || normalization == "" || keyID == "" {
		return "", "", "", "", contracts.ErrUnsafe
	}
	return strategyID, strategyVersion, normalization, keyID, nil
}

func newRecoveryBindingSet(req StreamingRecoveryRequest, settings string, key contracts.Value) (recoveryBindingSet, error) {
	planDigest, err := history.CanonicalPlanDigest(req.Plan)
	if err != nil {
		return recoveryBindingSet{}, err
	}
	strategyID, strategyVersion, normalization, keyID, err := planRecoveryBinding(req.Plan)
	if err != nil {
		return recoveryBindingSet{}, err
	}
	keyCheck, err := recoveryKeyCheck(key, keyID)
	if err != nil {
		return recoveryBindingSet{}, err
	}
	return recoveryBindingSet{
		keyCheck:          keyCheck,
		planContentDigest: planDigest, scopeDigest: recoveryScopeDigest(req.Plan),
		sourceSnapshot: req.Plan.SourceSnapshotID, targetID: req.Plan.TargetID,
		schemaDigest: req.Plan.SchemaDigest, configDigest: req.Plan.ConfigDigest,
		strategyID: strategyID, strategyVersion: strategyVersion,
		normalization: normalization, keyID: keyID, settingsDigest: settings,
	}, nil
}

// recoveryManifestFor builds the manifest of a new run. It contains identifiers,
// digests, counts and timestamps only.
func recoveryManifestFor(req contracts.StreamingApplyRequest, targetIdentity, settingsDigest string, key contracts.Value, now time.Time) (db.RecoveryManifest, error) {
	binding, err := newRecoveryBindingSet(StreamingRecoveryRequest{
		Protocol: req.Protocol, Plan: req.Plan, Approval: req.Approval,
		ExplicitApply: req.ExplicitApply, RunID: req.RunID,
		BatchSize: req.BatchSize, ReceiptPageSize: req.ReceiptPageSize, Budget: req.Budget,
	}, settingsDigest, key)
	if err != nil {
		return db.RecoveryManifest{}, err
	}
	return db.RecoveryManifest{
		RunID: req.RunID, Protocol: db.RecoveryProtocolRevision,
		StreamingProtocol: req.Protocol, ApprovalBinding: contracts.ApprovalBindingRevision,
		PlanID: req.Plan.ID, PlanContentDigest: binding.planContentDigest,
		ScopeDigest: binding.scopeDigest, SourceSnapshotDigest: binding.sourceSnapshot,
		TargetID: binding.targetID, TargetIdentity: targetIdentity,
		SchemaDigest: binding.schemaDigest, ConfigDigest: binding.configDigest,
		StrategyID: binding.strategyID, StrategyVersion: binding.strategyVersion,
		NormalizationVersion: binding.normalization, KeyID: binding.keyID,
		KeyCheck: binding.keyCheck, SettingsDigest: binding.settingsDigest, State: db.RecoveryStateNew,
		BusinessComplete: false, Validation: db.ValidationNone,
		CheckpointSequence: 0, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// openRecoveryForRun opens the outbox this run already uses and requires the
// recovery schema. It never installs or repairs anything.
func (e *Engine) openRecoveryForRun(ctx context.Context, req StreamingRecoveryRequest) (*db.Recovery, contracts.OutboxCapabilities, error) {
	opts, err := e.streamingOutboxOptions(contracts.StreamingApplyRequest{Protocol: req.Protocol, Budget: req.Budget})
	if err != nil {
		return nil, contracts.OutboxCapabilities{}, err
	}
	outbox, err := db.OpenOutbox(ctx, opts)
	if err != nil {
		return nil, contracts.OutboxCapabilities{}, err
	}
	available, err := outbox.RecoveryAvailable(ctx)
	if err != nil {
		_ = outbox.Close()
		return nil, contracts.OutboxCapabilities{}, err
	}
	if !available {
		_ = outbox.Close()
		return nil, contracts.OutboxCapabilities{}, contracts.ErrUnsupported
	}
	recovery, err := outbox.OpenRecovery(ctx)
	if err != nil {
		_ = outbox.Close()
		return nil, contracts.OutboxCapabilities{}, err
	}
	return recovery, outbox.Capabilities(), nil
}

// StreamingResume verifies an existing run and continues it. Every pre-write
// gate runs before a new business transaction exists.
func (e *Engine) StreamingResume(ctx context.Context, req StreamingRecoveryRequest) (contracts.StreamingApplySummary, RecoveryReport, error) {
	var summary contracts.StreamingApplySummary
	var report RecoveryReport
	if e == nil || ctx == nil {
		return summary, report, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return summary, report, err
	}
	if err := validateRecoveryStructure(req, e.cfg); err != nil {
		return summary, report, fmt.Errorf("validateRecoveryStructure: %w", err)
	}
	if err := e.checkReservedPlan(req.Plan); err != nil {
		return summary, report, fmt.Errorf("checkReservedPlan: %w", err)
	}
	if err := e.checkStreamingApprovalPlan(ctx, req.Plan, req.Approval); err != nil {
		return summary, report, fmt.Errorf("checkStreamingApproval: %w", err)
	}
	// Static preflight only. The ordinary full dry run assumes an unmasked
	// target, so it is replaced (not disabled) by the prefix/suffix recovery
	// verification below.
	if err := e.preflight(ctx, req.Plan); err != nil {
		return summary, report, fmt.Errorf("preflight: %w", err)
	}
	settingsDigest, err := recoverySettingsDigest(req.Budget, req.BatchSize)
	if err != nil {
		return summary, report, err
	}
	binding, err := newRecoveryBindingSet(req, settingsDigest, e.key)
	if err != nil {
		return summary, report, err
	}
	recovery, caps, err := e.openRecoveryForRun(ctx, req)
	if err != nil {
		return summary, report, fmt.Errorf("openRecovery: %w", err)
	}
	defer recovery.Close()
	if err := caps.Validate(req.Protocol); err != nil {
		return summary, report, err
	}
	if req.ReceiptPageSize > caps.MaxReceiptPage {
		return summary, report, contracts.ErrInvalid
	}
	manifest, err := recovery.Manifest(ctx, req.RunID)
	if err != nil {
		return summary, report, fmt.Errorf("manifest: %w", err)
	}
	report = recoveryReportFrom(req, manifest)
	report.Tables = len(req.Plan.Tables)
	if err := recovery.VerifyOutboxRun(ctx, manifest); err != nil {
		return summary, report, err
	}
	if err := e.verifyManifestBinding(manifest, binding, req); err != nil {
		return summary, report, err
	}
	if manifest.State == db.RecoveryStateConflict || manifest.State == db.RecoveryStateAborted {
		return summary, report, contracts.ErrConflict
	}
	last, err := verifyRecoveryProgress(ctx, recovery, manifest, req.ReceiptPageSize)
	if err != nil {
		if errors.Is(err, contracts.ErrConflict) {
			report.State = db.RecoveryStateConflict
		}
		return summary, report, err
	}
	// business_complete is only legitimate after the final table boundary.
	if !businessCompleteValid(last, len(req.Plan.Tables)) {
		report.State = db.RecoveryStateConflict
		return summary, report, contracts.ErrConflict
	}
	report.TableOrdinal = last.TableOrdinal
	report.RowsProcessed = last.RowsProcessed
	report.RowsCommitted = last.RowsCommitted
	report.CommittedBatches = last.CommittedBatches
	report.CheckpointSequence = last.Sequence
	report.BusinessComplete = manifest.BusinessComplete || last.BusinessComplete
	if err := e.verifyRecoveryTarget(ctx, req.Plan, last, req.Budget.MaxRowBytes); err != nil {
		_ = recovery.SetManifestState(context.Background(), req.RunID, manifest.CheckpointSequence,
			db.RecoveryStateConflict, manifest.BusinessComplete, manifest.Validation)
		report.State = db.RecoveryStateConflict
		return summary, report, fmt.Errorf("verifyRecoveryTarget: %w", err)
	}
	if manifest.State == db.RecoveryStateValidationFailed {
		return summary, report, contracts.ErrIncomplete
	}
	if manifest.State == db.RecoveryStateComplete && manifest.Validation == db.ValidationPassed {
		summary, err = e.recoveryValidate(ctx, req, manifest, last)
		if err != nil {
			return summary, report, err
		}
		report.State = db.RecoveryStateComplete
		report.Validation = db.ValidationPassed
		report.BusinessComplete = true
		report.AuditComplete = true
		report.ResumeEligible = true
		return summary, report, nil
	}
	if err := recovery.SetManifestState(ctx, req.RunID, manifest.CheckpointSequence,
		db.RecoveryStateRunning, manifest.BusinessComplete, db.ValidationNone); err != nil {
		return summary, report, err
	}
	run := &streamingRun{
		engine: e, outbox: recovery.Outbox, recovery: recovery,
		summary: &summary, batch: req.BatchSize, budget: req.Budget,
	}
	summary = contracts.StreamingApplySummary{
		Protocol: req.Protocol, PlanID: req.Plan.ID, RunID: req.RunID,
		ReceiptPageSize: req.ReceiptPageSize, State: contracts.RunStateRunning,
		Audit:        contracts.AuditStateConsistent,
		BatchesTotal: last.CommittedBatches, BatchesCommitted: last.CommittedBatches,
		RowsCommitted: last.RowsCommitted,
	}
	run.summary = &summary
	run.progress = last
	run.progress.RunID = req.RunID
	run.progress.BusinessComplete = manifest.BusinessComplete || last.BusinessComplete
	fail := func(cause error) (contracts.StreamingApplySummary, RecoveryReport, error) {
		summary = run.finishRecovery(req, cause)
		report.State, report.Validation = recoveryFailureState(cause)
		report.TableOrdinal = run.progress.TableOrdinal
		report.RowsProcessed = run.progress.RowsProcessed
		report.RowsCommitted = run.progress.RowsCommitted
		report.CommittedBatches = run.progress.CommittedBatches
		report.CheckpointSequence = run.progress.Sequence
		report.BusinessComplete = run.progress.BusinessComplete
		report.AuditComplete = false
		report.ResumeEligible = recoveryResumeEligible(report.State)
		return summary, report, cause
	}
	if !run.progress.BusinessComplete {
		if err := run.continueRecovery(ctx, req.Plan); err != nil {
			return fail(err)
		}
	}
	if err := testhooks.Fire("before_validate"); err != nil {
		return fail(err)
	}
	summary, err = e.recoveryValidate(ctx, req, manifest, run.progress)
	if err != nil {
		report.State = db.RecoveryStateValidationFailed
		report.Validation = db.ValidationFailed
		report.TableOrdinal = run.progress.TableOrdinal
		report.RowsProcessed = run.progress.RowsProcessed
		report.RowsCommitted = run.progress.RowsCommitted
		report.CommittedBatches = run.progress.CommittedBatches
		report.CheckpointSequence = run.progress.Sequence
		report.BusinessComplete = true
		report.ResumeEligible = false
		return summary, report, err
	}
	if err := testhooks.Fire("after_validate_before_complete"); err != nil {
		return fail(err)
	}
	if err := recovery.SetManifestState(ctx, req.RunID, run.progress.Sequence, db.RecoveryStateComplete, true, db.ValidationPassed); err != nil {
		return fail(err)
	}
	summary.State = contracts.RunStateComplete
	summary.Audit = contracts.AuditStateConsistent
	summary.BatchesTotal = summary.BatchesCommitted
	report.State = db.RecoveryStateComplete
	report.Validation = db.ValidationPassed
	report.TableOrdinal = run.progress.TableOrdinal
	report.RowsProcessed = run.progress.RowsProcessed
	report.RowsCommitted = run.progress.RowsCommitted
	report.CommittedBatches = run.progress.CommittedBatches
	report.CheckpointSequence = run.progress.Sequence
	report.BusinessComplete = true
	report.AuditComplete = true
	report.ResumeEligible = true
	return summary, report, nil
}

func recoveryFailureState(cause error) (db.RecoveryState, db.ValidationState) {
	switch {
	case errors.Is(cause, contracts.ErrCommitUnknown):
		return db.RecoveryStateCommitUnknown, db.ValidationNone
	case errors.Is(cause, contracts.ErrConflict):
		return db.RecoveryStateConflict, db.ValidationNone
	default:
		return db.RecoveryStateRecoverable, db.ValidationNone
	}
}

func (e *Engine) verifyManifestBinding(m db.RecoveryManifest, b recoveryBindingSet, req StreamingRecoveryRequest) error {
	// Key material is bound by a key-check value, not by the KeyID label. A
	// wrong key file must be refused before any write, even on a vacuous prefix.
	if m.KeyCheck != b.keyCheck {
		return contracts.ErrUnsafe
	}
	if m.Protocol != db.RecoveryProtocolRevision ||
		m.StreamingProtocol != contracts.StreamingProtocolRevision ||
		m.ApprovalBinding != contracts.ApprovalBindingRevision ||
		m.PlanID != req.Plan.ID ||
		m.PlanContentDigest != b.planContentDigest ||
		m.ScopeDigest != b.scopeDigest ||
		m.SourceSnapshotDigest != b.sourceSnapshot ||
		m.TargetID != b.targetID ||
		m.SchemaDigest != b.schemaDigest ||
		m.ConfigDigest != b.configDigest ||
		m.StrategyID != b.strategyID ||
		m.StrategyVersion != b.strategyVersion ||
		m.NormalizationVersion != b.normalization ||
		m.KeyID != b.keyID ||
		m.SettingsDigest != b.settingsDigest {
		return contracts.ErrConflict
	}
	identity, err := db.TargetIdentity(e.cfg.Target)
	if err != nil {
		return contracts.ErrUnsupported
	}
	if m.TargetIdentity != identity {
		return contracts.ErrConflict
	}
	return nil
}

// recoverableStrategies is the explicit allowlist of strategies whose output is
// a pure deterministic function of source+plan+key. A strategy that needs
// cross-process mapping state must not be resume-enabled silently.
var recoverableStrategies = map[string]bool{
	"null": true, "blank": true, "redact": true, "format_random": true,
	"fake_email": true, "cn_id_card": true, "cn_mobile": true, "cn_bank_card": true,
	"cn_uscc": true, "cn_postal_code": true, "cn_city": true, "cn_person_name": true,
	"cn_address": true, "cn_date": true,
}

func validateRecoverablePlan(plan contracts.Plan) error {
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if !recoverableStrategies[column.Strategy.ID] {
				return contracts.ErrUnsupported
			}
		}
	}
	return nil
}

func validateRecoveryStructure(req StreamingRecoveryRequest, cfg config.Runtime) error {
	apply := contracts.StreamingApplyRequest{
		Protocol: req.Protocol, Plan: req.Plan, Approval: req.Approval,
		ExplicitApply: req.ExplicitApply, RunID: req.RunID,
		BatchSize: req.BatchSize, ReceiptPageSize: req.ReceiptPageSize, Budget: req.Budget,
	}
	if err := validateStreamingStructure(apply, cfg); err != nil {
		return err
	}
	if err := req.Budget.ValidatePlan(req.Plan); err != nil {
		return err
	}
	if err := validateRecoverablePlan(req.Plan); err != nil {
		return err
	}
	return contracts.ValidateStreamingApproval(req.Plan, req.Approval, time.Now())
}

func recoveryReportFrom(req StreamingRecoveryRequest, m db.RecoveryManifest) RecoveryReport {
	return RecoveryReport{
		Protocol: m.Protocol, RunID: req.RunID, PlanID: req.Plan.ID,
		State: m.State, Validation: m.Validation,
		BusinessComplete: m.BusinessComplete,
		AuditComplete:    m.State == db.RecoveryStateComplete && m.Validation == db.ValidationPassed,
		ResumeEligible:   recoveryResumeEligible(m.State),
	}
}

// recoveryResumeEligible is the single eligibility rule shared by status and
// the resume report. validation_failed is terminal for resume.
func businessCompleteValid(last db.RecoveryProgress, tables int) bool {
	return !last.BusinessComplete || last.TableOrdinal == tables
}

func recoveryResumeEligible(state db.RecoveryState) bool {
	return state != db.RecoveryStateConflict && state != db.RecoveryStateAborted &&
		state != db.RecoveryStateValidationFailed
}

// verifyRecoveryProgress re-derives the committed range from the authoritative
// progress log, checks monotonic transitions and cross-checks the receipts. Both
// reads are page-bounded.
func verifyRecoveryProgress(ctx context.Context, recovery *db.Recovery, manifest db.RecoveryManifest, pageSize int) (db.RecoveryProgress, error) {
	if pageSize < 1 {
		return db.RecoveryProgress{}, contracts.ErrInvalid
	}
	last := db.RecoveryProgress{RunID: manifest.RunID, TableOrdinal: 0}
	var expected int64 = 1
	for {
		page, done, err := recovery.ProgressPage(ctx, manifest.RunID, last.Sequence, pageSize)
		if err != nil {
			return db.RecoveryProgress{}, err
		}
		for _, p := range page {
			if p.Sequence != expected {
				return db.RecoveryProgress{}, contracts.ErrConflict
			}
			if err := checkProgressTransition(last, p); err != nil {
				return db.RecoveryProgress{}, err
			}
			last = p
			expected++
		}
		if done {
			break
		}
		if len(page) == 0 {
			return db.RecoveryProgress{}, contracts.ErrConflict
		}
	}
	if expected-1 != manifest.CheckpointSequence {
		return db.RecoveryProgress{}, contracts.ErrConflict
	}
	if last.BusinessComplete != manifest.BusinessComplete {
		return db.RecoveryProgress{}, contracts.ErrConflict
	}
	if manifest.CheckpointSequence == 0 && manifest.BusinessComplete {
		return db.RecoveryProgress{}, contracts.ErrConflict
	}
	if err := verifyRecoveryReceipts(ctx, recovery, manifest.RunID, last, pageSize); err != nil {
		return db.RecoveryProgress{}, err
	}
	return last, nil
}

// checkProgressTransition applies the same monotonic rules the database enforces
// on insert. It is a second, independent check of a possibly tampered log.
func checkProgressTransition(prev, p db.RecoveryProgress) error {
	if p.Kind != "batch" && p.Kind != "progress" && p.Kind != "boundary" {
		return contracts.ErrConflict
	}
	if (p.Kind == "batch") != (p.BatchID != "" && p.AffectedRows > 0) {
		return contracts.ErrConflict
	}
	if p.Kind != "batch" && (p.BatchID != "" || p.AffectedRows != 0) {
		return contracts.ErrConflict
	}
	if p.BusinessComplete && p.Kind != "boundary" {
		return contracts.ErrConflict
	}
	wantCommitted := prev.RowsCommitted
	wantBatches := prev.CommittedBatches
	if p.Kind == "batch" {
		wantCommitted += p.AffectedRows
		wantBatches++
	}
	if p.RowsCommitted != wantCommitted || p.CommittedBatches != wantBatches {
		return contracts.ErrConflict
	}
	switch {
	case p.TableOrdinal == prev.TableOrdinal:
		if p.Kind == "boundary" || p.RowsProcessed < prev.RowsProcessed {
			return contracts.ErrConflict
		}
	case p.TableOrdinal == prev.TableOrdinal+1 && p.Kind == "boundary":
		if p.RowsProcessed != 0 {
			return contracts.ErrConflict
		}
	default:
		return contracts.ErrConflict
	}
	return nil
}

func verifyRecoveryReceipts(ctx context.Context, recovery *db.Recovery, runID string, last db.RecoveryProgress, pageSize int) error {
	reader, err := recovery.Outbox.Reader(runID)
	if err != nil {
		return err
	}
	defer reader.Close()
	cursor := &receiptCursor{reader: reader, pageSize: pageSize}
	var after int64
	var expectedBatches, rows int64
	for {
		page, done, err := recovery.ProgressPage(ctx, runID, after, pageSize)
		if err != nil {
			return err
		}
		for _, p := range page {
			if p.Kind != "batch" {
				continue
			}
			receipt, ok, err := cursor.next(ctx)
			if err != nil {
				return err
			}
			if !ok {
				return contracts.ErrConflict
			}
			// Per-event reconciliation, not just an aggregate: the receipt of
			// this ordinal must name this batch, table-independent row count and
			// run-scoped batch id.
			if receipt.Ordinal != p.CommittedBatches-1 || receipt.Rows != p.AffectedRows ||
				receipt.BatchID != p.BatchID || receipt.Outcome != contracts.BatchCommitted {
				return contracts.ErrConflict
			}
			expectedBatches++
			rows += receipt.Rows
			after = p.Sequence
		}
		if done {
			break
		}
		if len(page) > 0 {
			after = page[len(page)-1].Sequence
		}
	}
	if _, ok, err := cursor.next(ctx); err != nil {
		return err
	} else if ok {
		// A receipt without a matching batch progress event is tamper evidence.
		return contracts.ErrConflict
	}
	if expectedBatches != last.CommittedBatches || rows != last.RowsCommitted {
		return contracts.ErrConflict
	}
	return nil
}

// receiptCursor pulls receipts one at a time from bounded pages.
type receiptCursor struct {
	reader   *db.OutboxReceiptReader
	pageSize int
	page     contracts.ReceiptPage
	index    int
	done     bool
}

func (c *receiptCursor) next(ctx context.Context) (contracts.StreamingBatchReceipt, bool, error) {
	for {
		if c.index < len(c.page.Receipts) {
			receipt := c.page.Receipts[c.index]
			c.index++
			return receipt, true, nil
		}
		if c.done {
			return contracts.StreamingBatchReceipt{}, false, nil
		}
		page, err := c.reader.Page(ctx, c.pageSize)
		if err != nil {
			return contracts.StreamingBatchReceipt{}, false, err
		}
		c.page, c.index = page, 0
		c.done = page.Done
	}
}

// verifyRecoveryTarget proves that the committed prefix is exactly the
// deterministic output and the uncommitted suffix is still the original source.
func (e *Engine) verifyRecoveryTarget(ctx context.Context, plan contracts.Plan, last db.RecoveryProgress, maxRowBytes int64) error {
	if last.TableOrdinal < 0 || last.TableOrdinal > len(plan.Tables) {
		return contracts.ErrConflict
	}
	if last.TableOrdinal == len(plan.Tables) && !last.BusinessComplete {
		return contracts.ErrConflict
	}
	for ti, table := range plan.Tables {
		if ti > last.TableOrdinal {
			// Unvisited tables are checked page by page before any write.
			return nil
		}
		mode := "unmasked"
		prefix := int64(0)
		switch {
		case ti < last.TableOrdinal:
			mode = "masked"
		case ti == last.TableOrdinal:
			mode = "prefix"
			prefix = last.RowsProcessed
		}
		if err := e.verifyTableRegion(ctx, table, mode, prefix, maxRowBytes); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) verifyTableRegion(ctx context.Context, table contracts.TablePlan, mode string, prefix, maxRowBytes int64) error {
	cols := planColumns(table)
	var after []contracts.Value
	var index int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		request := contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: maxPage,
		}
		left, err := pageWithin(ctx, e.source, request, maxRowBytes)
		if err != nil {
			return err
		}
		right, err := pageWithin(ctx, e.target, request, maxRowBytes)
		if err != nil {
			return err
		}
		if len(left.Rows) != len(right.Rows) || left.Done != right.Done || len(left.Rows) == 0 {
			return contracts.ErrConflict
		}
		for i := range left.Rows {
			if err := e.verifyRecoveryRow(ctx, table, left.Rows[i], right.Rows[i], mode, index < prefix); err != nil {
				return err
			}
			index++
		}
		if left.Done {
			break
		}
		if len(left.Next) != len(table.PrimaryKey) || len(right.Next) != len(table.PrimaryKey) {
			return contracts.ErrConflict
		}
		for i := range left.Next {
			if left.Next[i] != right.Next[i] {
				return contracts.ErrConflict
			}
		}
		after = append([]contracts.Value(nil), left.Next...)
	}
	if mode != "unmasked" && index < prefix {
		return contracts.ErrConflict
	}
	return nil
}

func (e *Engine) verifyRecoveryRow(ctx context.Context, table contracts.TablePlan, source, target contracts.Row, mode string, inPrefix bool) error {
	for _, name := range table.PrimaryKey {
		s, a := source[name]
		t, b := target[name]
		if !a || !b || s.IsNull() || s != t {
			return contracts.ErrConflict
		}
	}
	masked := mode == "masked" || (mode == "prefix" && inPrefix)
	for _, col := range table.Columns {
		name := col.Column.Column
		s, a := source[name]
		t, b := target[name]
		if !a || !b {
			return contracts.ErrConflict
		}
		if !masked {
			if s != t {
				return contracts.ErrConflict
			}
			continue
		}
		if s.IsNull() {
			if !t.IsNull() {
				return contracts.ErrConflict
			}
			continue
		}
		expected, err := e.maskColumn(ctx, s, col)
		if err != nil {
			return err
		}
		if t != expected {
			return contracts.ErrConflict
		}
	}
	return nil
}

// reconstructCursor re-scans the immutable source and returns the key after
// exactly processed rows. tableComplete reports that the whole table is already
// processed.
func (e *Engine) reconstructCursor(ctx context.Context, table contracts.TablePlan, processed, maxRowBytes int64) ([]contracts.Value, bool, error) {
	if processed == 0 {
		return nil, false, nil
	}
	cols := planColumns(table)
	var after []contracts.Value
	var count int64
	for {
		request := contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: maxPage,
		}
		page, err := pageWithin(ctx, e.source, request, maxRowBytes)
		if err != nil {
			return nil, false, err
		}
		if len(page.Rows) == 0 {
			return nil, false, contracts.ErrConflict
		}
		for i := range page.Rows {
			count++
			if count == processed {
				last := page.Rows[i]
				keys := make([]contracts.Value, len(table.PrimaryKey))
				for k, name := range table.PrimaryKey {
					value, ok := last[name]
					if !ok || value.IsNull() {
						return nil, false, contracts.ErrConflict
					}
					keys[k] = value
				}
				// Only when this is the final row of the page and the page is the
				// final page is the whole table processed. Otherwise the remaining
				// rows of this same page are still unprocessed.
				if page.Done && i == len(page.Rows)-1 {
					return nil, true, nil
				}
				return keys, false, nil
			}
		}
		if page.Done {
			// The table ended before the claimed processed row count.
			return nil, false, contracts.ErrConflict
		}
		after = append([]contracts.Value(nil), page.Next...)
	}
}

// continueRecovery continues from the verified checkpoint. Tables before the
// checkpoint are never re-read for writing.
func (r *streamingRun) continueRecovery(ctx context.Context, plan contracts.Plan) error {
	start := r.progress.TableOrdinal
	for ti := start; ti < len(plan.Tables); ti++ {
		table := plan.Tables[ti]
		var after []contracts.Value
		processed := int64(0)
		tableComplete := false
		if ti == start {
			processed = r.progress.RowsProcessed
			var err error
			after, tableComplete, err = r.engine.reconstructCursor(ctx, table, processed, r.budget.MaxRowBytes)
			if err != nil {
				return err
			}
		}
		if !tableComplete {
			if err := r.applyRecoveryTable(ctx, ti, table, after, processed); err != nil {
				return err
			}
			processed = r.progress.RowsProcessed
			if processed < 0 {
				return contracts.ErrConflict
			}
		}
		if err := r.commitBoundary(ctx, ti, len(plan.Tables)); err != nil {
			return err
		}
	}
	return nil
}

func (r *streamingRun) applyRecoveryTable(ctx context.Context, ti int, table contracts.TablePlan, after []contracts.Value, processed int64) error {
	rows, err := r.countTable(ctx, table)
	if err != nil {
		return err
	}
	cols := planColumns(table)
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
		pending := make([]contracts.RowChange, 0, len(changes))
		for _, change := range changes {
			if assignsNothing(change) {
				continue
			}
			pending = append(pending, change)
		}
		processed += int64(len(left.Rows))
		if err := testhooks.Fire("before_batch"); err != nil {
			return err
		}
		if len(pending) > 0 {
			if err := r.writeRecoveryBatch(ctx, ti, table, pending, processed, pageBytes); err != nil {
				return err
			}
		} else {
			if err := r.writeRecoveryProgress(ctx, ti, processed); err != nil {
				return err
			}
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

func (r *streamingRun) writeRecoveryBatch(ctx context.Context, ti int, table contracts.TablePlan,
	changes []contracts.RowChange, processed, pageBytes int64) error {
	if err := checkDiskAdmission(r.engine.cfg.Target, pageBytes, r.budget.MaxDiskBytes); err != nil {
		return err
	}
	sequence := r.progress.CommittedBatches
	receipt := contracts.StreamingBatchReceipt{
		Table: table.Table, BatchID: batchID(r.summary.RunID, sequence),
		Ordinal: sequence, Rows: int64(len(changes)), Outcome: contracts.BatchCommitted,
	}
	progress := db.RecoveryProgress{
		RunID: r.summary.RunID, Sequence: r.progress.Sequence + 1, Kind: "batch",
		TableOrdinal: ti, RowsProcessed: processed,
		RowsCommitted:    r.progress.RowsCommitted + int64(len(changes)),
		CommittedBatches: r.progress.CommittedBatches + 1,
		BatchID:          receipt.BatchID, AffectedRows: int64(len(changes)),
	}
	tx, err := r.recovery.BeginRecoveryBatch(ctx, r.summary.RunID, table, receipt, progress)
	if err != nil {
		if errors.Is(err, contracts.ErrUnsafe) || errors.Is(err, contracts.ErrInvalid) ||
			errors.Is(err, contracts.ErrNotFound) || errors.Is(err, contracts.ErrConflict) {
			return err
		}
		return r.classify(contracts.BatchFailedBeforeCommit, err)
	}
	n, uerr := tx.Update(ctx, changes)
	if uerr != nil {
		_ = rollbackBounded(tx)
		return r.classify(contracts.ClassifyBatch(false, nil, nil), uerr)
	}
	if n != int64(len(changes)) {
		_ = rollbackBounded(tx)
		return r.classify(contracts.BatchFailedBeforeCommit, contracts.ErrConflict)
	}
	cerr := tx.Commit(ctx)
	outcome := contracts.ClassifyBatch(true, cerr, nil)
	if cerr != nil {
		if errors.Is(cerr, contracts.ErrCommitUnknown) {
			r.markCommitUnknown(ctx)
		}
		return r.classify(outcome, cerr)
	}
	r.progress = progress
	r.summary.BatchesTotal++
	r.summary.BatchesCommitted++
	r.summary.RowsCommitted += n
	return checkDiskAfterCommit(r.engine.cfg.Target, r.budget.MaxDiskBytes)
}

func (r *streamingRun) writeRecoveryProgress(ctx context.Context, ti int, processed int64) error {
	progress := db.RecoveryProgress{
		RunID: r.summary.RunID, Sequence: r.progress.Sequence + 1, Kind: "progress",
		TableOrdinal: ti, RowsProcessed: processed,
		RowsCommitted: r.progress.RowsCommitted, CommittedBatches: r.progress.CommittedBatches,
	}
	if err := r.recovery.CommitProgress(ctx, r.summary.RunID, progress); err != nil {
		if errors.Is(err, contracts.ErrCommitUnknown) {
			r.markCommitUnknown(ctx)
		}
		return err
	}
	r.progress = progress
	return nil
}

func (r *streamingRun) commitBoundary(ctx context.Context, ti, tables int) error {
	progress := db.RecoveryProgress{
		RunID: r.summary.RunID, Sequence: r.progress.Sequence + 1, Kind: "boundary",
		TableOrdinal: ti + 1, RowsProcessed: 0,
		RowsCommitted: r.progress.RowsCommitted, CommittedBatches: r.progress.CommittedBatches,
		BusinessComplete: ti+1 == tables,
	}
	if err := r.recovery.CommitProgress(ctx, r.summary.RunID, progress); err != nil {
		if errors.Is(err, contracts.ErrCommitUnknown) {
			r.markCommitUnknown(ctx)
		}
		return err
	}
	r.progress = progress
	return nil
}

func (r *streamingRun) markCommitUnknown(ctx context.Context) {
	cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.recovery.SetManifestState(cleanup, r.summary.RunID, r.progress.Sequence,
		db.RecoveryStateCommitUnknown, r.progress.BusinessComplete, db.ValidationNone)
}

// recoveryValidate runs the strict read-only validation after business
// completion and records the truthful terminal validation state.
func (e *Engine) recoveryValidate(ctx context.Context, req StreamingRecoveryRequest,
	manifest db.RecoveryManifest, last db.RecoveryProgress) (contracts.StreamingApplySummary, error) {
	summary := contracts.StreamingApplySummary{
		Protocol: req.Protocol, PlanID: req.Plan.ID, RunID: req.RunID,
		ReceiptPageSize: req.ReceiptPageSize, State: contracts.RunStateRunning,
		Audit:        contracts.AuditStateConsistent,
		BatchesTotal: last.CommittedBatches, BatchesCommitted: last.CommittedBatches,
		RowsCommitted: last.RowsCommitted,
	}
	report, verr := e.validateWithin(ctx, contracts.ValidateRequest{Plan: req.Plan, Strict: true}, req.Budget.MaxRowBytes)
	var overflow bool
	summary.IssueCounts, overflow = boundedIssues(report.Issues, req.Budget.MaxIssueCounts)
	if overflow || verr != nil || !report.Passed(true) {
		summary.State = contracts.RunStateIncomplete
		summary.Audit = contracts.AuditStateIncomplete
		_ = e.markValidationFailed(ctx, req, manifest, last.Sequence)
		if verr != nil {
			return summary, verr
		}
		return summary, contracts.ErrIncomplete
	}
	summary.State = contracts.RunStateComplete
	summary.Audit = contracts.AuditStateConsistent
	summary.BatchesTotal = summary.BatchesCommitted
	return summary, nil
}

func (e *Engine) markValidationFailed(ctx context.Context, req StreamingRecoveryRequest, manifest db.RecoveryManifest, sequence int64) error {
	recovery, _, err := e.openRecoveryForRun(ctx, req)
	if err != nil {
		return err
	}
	defer recovery.Close()
	return recovery.SetManifestState(ctx, req.RunID, sequence, db.RecoveryStateValidationFailed, true, db.ValidationFailed)
}

func (r *streamingRun) finishRecovery(req StreamingRecoveryRequest, cause error) contracts.StreamingApplySummary {
	summary := *r.summary
	state, validation := recoveryFailureState(cause)
	if !errors.Is(cause, contracts.ErrCommitUnknown) {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = r.recovery.SetManifestState(cleanup, req.RunID, r.progress.Sequence, state,
			r.progress.BusinessComplete, validation)
		cancel()
	}
	if summary.State == contracts.RunStateRunning {
		if summary.BatchesUnknown > 0 {
			summary.State = contracts.RunStateCommitUnknown
			summary.Audit = contracts.AuditStateIncomplete
		} else {
			summary.State = contracts.RunStateIncomplete
		}
	}
	return summary
}
