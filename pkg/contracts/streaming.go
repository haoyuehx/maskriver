package contracts

import (
	"context"
	"errors"
	"time"
)

// StreamingProtocolRevision versions the additive M2 streaming Apply surface.
//
// It is deliberately independent of APIRevision. Plan, Plan.ID, the signed
// approval record, ApplyRequest/ApplyReport and the legacy execution path stay
// m1a-v1 and are not reinterpreted by this surface. A caller opts in by naming
// this protocol explicitly; there is no automatic upgrade or fallback.
//
// Choosing this protocol does not prove bounded memory, durability, crash
// consistency or tamper evidence. Those claims require the implementation and
// evidence gates recorded in the Issue #13 architecture decision record.
const StreamingProtocolRevision = "m2-stream-v1"

// ApprovalBindingRevision versions the streaming approval binding. It is not
// the signed history record format and it does not replace signature
// verification: the history store remains authoritative for Ed25519 signatures,
// expiry clocks and revocation. This binding only proves that a verified record
// was bound to this protocol and to exactly this plan.
const ApprovalBindingRevision = "m2-approval-binding-v1"

// RunState is the terminal or in-progress state reported by the streaming path
// for one run. It never reports success without a consistent local receipt
// state; see StreamingApplySummary.Validate.
type RunState uint8

const (
	RunStateUnknown RunState = iota
	RunStateRunning
	RunStateComplete
	RunStateIncomplete
	RunStateCommitUnknown
	RunStateAuditIncomplete
)

// Valid reports whether s is a defined run state other than RunStateUnknown.
func (s RunState) Valid() bool {
	switch s {
	case RunStateRunning, RunStateComplete, RunStateIncomplete, RunStateCommitUnknown, RunStateAuditIncomplete:
		return true
	default:
		return false
	}
}

// BatchOutcome is the per-batch classification. The distinction between a
// confirmed commit, an unknown commit, a known pre-commit failure and a missing
// local receipt is part of the contract and must not be collapsed.
type BatchOutcome uint8

const (
	BatchOutcomeUnknown BatchOutcome = iota
	// BatchCommitted means Commit returned success and the same-transaction
	// receipt is present. It does not by itself prove power-loss durability.
	BatchCommitted
	// BatchCommitUnknown means the underlying Commit was attempted and its
	// outcome is ambiguous. It must never be retried automatically.
	BatchCommitUnknown
	// BatchFailedBeforeCommit means a known failure before the commit attempt.
	BatchFailedBeforeCommit
	// BatchAuditIncomplete means data may be committed but the local receipt
	// state is not confirmed (for example a post-commit sink failure). It can
	// never be reported as complete success.
	BatchAuditIncomplete
)

// Valid reports whether o is a defined outcome other than the zero value.
func (o BatchOutcome) Valid() bool {
	switch o {
	case BatchCommitted, BatchCommitUnknown, BatchFailedBeforeCommit, BatchAuditIncomplete:
		return true
	default:
		return false
	}
}

// CommitClassification classifies a WriteTx.Commit result for the streaming
// path. Implementations must return ErrCommitUnknown only when the outcome is
// genuinely ambiguous; every other non-nil error is a known failure.
type CommitClassification uint8

const (
	CommitNotAttempted CommitClassification = iota
	CommitSucceeded
	CommitOutcomeUnknown
	CommitFailed
)

// ClassifyCommit maps a Commit error to the streaming classification. It does
// not inspect driver text, so it cannot invent a confirmed or unknown outcome.
func ClassifyCommit(err error) CommitClassification {
	switch {
	case err == nil:
		return CommitSucceeded
	case errors.Is(err, ErrCommitUnknown):
		return CommitOutcomeUnknown
	default:
		return CommitFailed
	}
}

// ClassifyBatch combines the commit classification with the local receipt
// delivery result. A committed batch whose receipt is not confirmed is
// BatchAuditIncomplete, never BatchCommitted; an unknown commit stays unknown
// even if receipt delivery also failed.
func ClassifyBatch(commit CommitClassification, receiptErr error) BatchOutcome {
	switch commit {
	case CommitSucceeded:
		if receiptErr != nil {
			return BatchAuditIncomplete
		}
		return BatchCommitted
	case CommitOutcomeUnknown:
		return BatchCommitUnknown
	case CommitFailed:
		return BatchFailedBeforeCommit
	default:
		return BatchOutcomeUnknown
	}
}

// AuditState describes the local receipt consistency of a streaming run. It is
// not tamper evidence and not a durability proof.
type AuditState uint8

const (
	AuditStateUnknown AuditState = iota
	// AuditStateConsistent means every confirmed batch has a confirmed local
	// same-transaction receipt. It does not imply an independent witness.
	AuditStateConsistent
	// AuditStateIncomplete means at least one batch lacks a confirmed receipt,
	// or the store could not be verified.
	AuditStateIncomplete
)

// StreamingBatchReceipt is one versioned receipt delivered through a bounded
// page or an internal sink. It carries identifiers, counts and classification
// only: never source values, replacements, primary-key values, masking keys or
// DSNs. BatchID is a run-scoped correlation identity, not a resume token and not
// an exactly-once guarantee.
type StreamingBatchReceipt struct {
	Table   TableRef
	BatchID string
	Ordinal int64
	Rows    int64
	Outcome BatchOutcome
}

// ReceiptPage is one bounded page of receipts. Page size is bounded by the
// caller's budget and by the outbox capability; the contract never returns an
// unbounded accumulated slice. NextOrdinal is the ordinal the next page starts
// at and is only meaningful when Done is false.
type ReceiptPage struct {
	Receipts    []StreamingBatchReceipt
	NextOrdinal int64
	Done        bool
}

// ReceiptReader reads receipts in bounded pages. Page must not retain receipts
// beyond the requested limit and must not materialize the complete run.
type ReceiptReader interface {
	Page(ctx context.Context, limit int) (ReceiptPage, error)
	Close() error
}

// ReceiptSink is an internal, trusted, synchronous receipt writer used on the
// commit path. It is not a user-supplied callback: an arbitrary callback could
// ignore context, block forever, re-enter or panic. Implementations must honour
// context cancellation, must not retain the receipt after return, must not
// silently drop receipts, and must surface delivery failure so the run becomes
// audit-incomplete. This round defines the contract only; no production sink is
// implemented.
type ReceiptSink interface {
	Deliver(ctx context.Context, receipt StreamingBatchReceipt) error
	Flush(ctx context.Context) error
}

// OutboxCapabilities declares what a receipt store/provider can actually do.
// The streaming path fails closed unless every required capability is present
// and the protocol and page bound match.
type OutboxCapabilities struct {
	// Preprovisioned means the audit schema already exists at a compatible
	// version; the Apply path performs no DDL.
	Preprovisioned bool
	// SameDatabase means receipts live in the same target database (and the
	// same transaction) as business writes.
	SameDatabase bool
	// Transactional means the receipt write participates in the business
	// transaction and is not a best-effort side write.
	Transactional bool
	// SupportsCommitUnknown means the implementation distinguishes an ambiguous
	// commit from a known failure instead of guessing.
	SupportsCommitUnknown bool
	SchemaVersion         string
	ProtocolVersions      []string
	MaxReceiptPage        int
}

// Supports reports whether the capability set names protocol and offers a
// positive receipt-page bound.
func (c OutboxCapabilities) Supports(protocol string) bool {
	if !c.Preprovisioned || !c.SameDatabase || !c.Transactional || !c.SupportsCommitUnknown || c.SchemaVersion == "" || c.MaxReceiptPage < 1 {
		return false
	}
	for _, candidate := range c.ProtocolVersions {
		if candidate == protocol {
			return true
		}
	}
	return false
}

// Validate checks the capability declaration against a required protocol. A
// missing schema version or a non-transactional backend is an unsupported
// capability, not a malformed request.
func (c OutboxCapabilities) Validate(protocol string) error {
	if c.MaxReceiptPage < 1 {
		return ErrInvalid
	}
	if !c.Supports(protocol) {
		return ErrUnsupported
	}
	return nil
}

// StreamingApproval is the protocol-bound form of a verified approval record.
// It mirrors only the binding fields needed to prove that the record was issued
// for exactly this plan and this protocol. Signature verification, clock skew,
// revocation storage and the reviewer identity remain the history store's
// responsibility; callers must not treat this value as authenticity.
type StreamingApproval struct {
	BindingRevision  string
	PlanID           string
	SourceSnapshotID string
	TargetID         string
	SchemaDigest     string
	ConfigDigest     string
	StrategyID       string
	StrategyVersion  string
	KeyID            string
	Scopes           []StreamingScope
	IssuedAt         time.Time
	ExpiresAt        time.Time
	Revoked          bool
}

// StreamingScope is one reviewed table/column target in plan order.
type StreamingScope struct {
	Table  TableRef
	Column string
}

// ResourceBudget declares the hard limits a streaming run must respect. This
// type carries no approved numeric value: every field is supplied by the caller
// and validated, and WP3 may not enable production until measured values are
// approved. MaxQueue is the internal sink queue capacity; 0 means synchronous
// delivery. There is no "unbounded" encoding.
type ResourceBudget struct {
	MaxBatchSize        int
	MaxWorkers          int
	MaxRowBytes         int64
	MaxColumns          int
	MaxTables           int
	MaxHistoryEntries   int
	MaxHistoryNameBytes int64
	MaxReceiptPage      int
	MaxQueue            int
	MaxProcessBytes     int64
	MaxDiskBytes        int64
}

// Validate requires every budget field to be explicitly supplied. A zero or
// negative value is not a default and must not silently mean unlimited.
func (b ResourceBudget) Validate() error {
	if b.MaxBatchSize <= 0 || b.MaxWorkers <= 0 || b.MaxRowBytes <= 0 ||
		b.MaxColumns <= 0 || b.MaxTables <= 0 || b.MaxHistoryEntries <= 0 ||
		b.MaxHistoryNameBytes <= 0 || b.MaxReceiptPage <= 0 ||
		b.MaxQueue < 0 || b.MaxProcessBytes <= 0 || b.MaxDiskBytes <= 0 {
		return ErrInvalid
	}
	return nil
}

// ValidatePlan checks the plan shape against the declared table/column caps.
func (b ResourceBudget) ValidatePlan(plan Plan) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if len(plan.Tables) > b.MaxTables {
		return ErrInvalid
	}
	columns := 0
	for _, table := range plan.Tables {
		columns += len(table.Columns)
	}
	if columns > b.MaxColumns {
		return ErrInvalid
	}
	return nil
}

// StreamingApplyRequest is the versioned request for the bounded streaming
// path. It consumes the existing m1a-v1 Plan plus a protocol-bound approval; it
// does not define a new Plan revision and does not change Plan.ID.
type StreamingApplyRequest struct {
	Protocol        string
	Plan            Plan
	Approval        StreamingApproval
	ExplicitApply   bool
	RunID           string
	BatchSize       int
	ReceiptPageSize int
	Budget          ResourceBudget
	Outbox          OutboxCapabilities
}

// Validate checks structural and binding invariants only. It does not approve a
// run, connect to a database, verify an Ed25519 signature, or prove that the
// outbox exists. Unsupported protocols and cross-protocol approvals fail closed.
func (r StreamingApplyRequest) Validate(now time.Time) error {
	if r.Protocol != StreamingProtocolRevision {
		return ErrUnsupported
	}
	if !r.ExplicitApply {
		return ErrUnsafe
	}
	if !ValidRunID(r.RunID) {
		return ErrInvalid
	}
	if err := r.Budget.Validate(); err != nil {
		return err
	}
	if err := r.Plan.ValidateShape(); err != nil {
		return err
	}
	if err := r.Budget.ValidatePlan(r.Plan); err != nil {
		return err
	}
	if r.BatchSize <= 0 || r.BatchSize > r.Budget.MaxBatchSize {
		return ErrInvalid
	}
	if r.ReceiptPageSize <= 0 || r.ReceiptPageSize > r.Budget.MaxReceiptPage {
		return ErrInvalid
	}
	if err := r.Outbox.Validate(r.Protocol); err != nil {
		return err
	}
	if r.ReceiptPageSize > r.Outbox.MaxReceiptPage {
		return ErrInvalid
	}
	return ValidateStreamingApproval(r.Plan, r.Approval, now)
}

// StreamingApplySummary is the bounded result of a streaming run. It contains no
// receipt slice: receipts are read through ReceiptReader or delivered to the
// internal sink, so the summary does not grow with the number of batches.
type StreamingApplySummary struct {
	Protocol         string
	PlanID           string
	RunID            string
	State            RunState
	Audit            AuditState
	BatchesCommitted int64
	BatchesUnknown   int64
	BatchesFailed    int64
	RowsCommitted    int64
	ReceiptPageSize  int
	Issues           []Issue
}

// Validate rejects contradictory summaries. In particular a complete run must
// have consistent local receipts and no unknown or failed batches, so a missing
// receipt or ambiguous commit can never be reported as success.
func (s StreamingApplySummary) Validate() error {
	if s.Protocol != StreamingProtocolRevision || s.PlanID == "" || !ValidRunID(s.RunID) || !s.State.Valid() {
		return ErrInvalid
	}
	if s.BatchesCommitted < 0 || s.BatchesUnknown < 0 || s.BatchesFailed < 0 || s.RowsCommitted < 0 || s.ReceiptPageSize <= 0 {
		return ErrInvalid
	}
	if s.State == RunStateComplete && s.Audit != AuditStateConsistent {
		return ErrIncomplete
	}
	if s.State == RunStateComplete && (s.BatchesUnknown != 0 || s.BatchesFailed != 0) {
		return ErrIncomplete
	}
	if s.Audit == AuditStateConsistent && s.BatchesUnknown != 0 {
		return ErrIncomplete
	}
	for _, issue := range s.Issues {
		if issue.Code == "" {
			return ErrInvalid
		}
	}
	return nil
}

// ValidateReceiptPage checks page bounds and sequence continuity. expectedNext
// is the ordinal the page must start at; the next page continues from
// NextOrdinal. Skipped, decreasing or oversized pages are rejected.
func ValidateReceiptPage(page ReceiptPage, expectedNext int64, limit int) error {
	if limit < 1 || len(page.Receipts) > limit || expectedNext < 0 {
		return ErrInvalid
	}
	next := expectedNext
	for _, receipt := range page.Receipts {
		if !validTable(receipt.Table) || receipt.BatchID == "" || receipt.Rows < 0 || !receipt.Outcome.Valid() {
			return ErrInvalid
		}
		if receipt.Ordinal != next {
			return ErrConflict
		}
		next++
	}
	if len(page.Receipts) > 0 && page.NextOrdinal != next {
		return ErrConflict
	}
	if len(page.Receipts) == 0 && !page.Done {
		return ErrInvalid
	}
	return nil
}

// ValidRunID reports whether id is a 128-bit run identity encoded as 32
// lowercase hexadecimal characters.
func ValidRunID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ValidateStreamingApproval proves that approval is bound to exactly this plan
// under the streaming protocol. It is a binding check only; the caller must have
// already verified the record's signature, reviewer, revocation state and clock
// through the history store. A legacy approval without a streaming binding is
// rejected, so an m1a-v1 authorization is never silently upgraded.
func ValidateStreamingApproval(plan Plan, approval StreamingApproval, now time.Time) error {
	if approval.BindingRevision != ApprovalBindingRevision {
		return ErrUnsupported
	}
	if err := plan.ValidateShape(); err != nil {
		return err
	}
	if approval.PlanID != plan.ID ||
		approval.SourceSnapshotID != plan.SourceSnapshotID ||
		approval.TargetID != plan.TargetID ||
		approval.SchemaDigest != plan.SchemaDigest ||
		approval.ConfigDigest != plan.ConfigDigest {
		return ErrUnsafe
	}
	strategyID, strategyVersion, keyID, err := planStrategy(plan)
	if err != nil {
		return err
	}
	if approval.StrategyID != strategyID || approval.StrategyVersion != strategyVersion || approval.KeyID != keyID {
		return ErrUnsafe
	}
	if approval.IssuedAt.IsZero() || approval.ExpiresAt.IsZero() || !approval.ExpiresAt.After(approval.IssuedAt) {
		return ErrInvalid
	}
	if now.Before(approval.IssuedAt) || !now.Before(approval.ExpiresAt) {
		return ErrUnsafe
	}
	if approval.Revoked {
		return ErrUnsafe
	}
	if len(approval.Scopes) != planScopeCount(plan) {
		return ErrUnsafe
	}
	index := 0
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			scope := approval.Scopes[index]
			if scope.Table != table.Table || scope.Column != column.Column.Column {
				return ErrUnsafe
			}
			index++
		}
	}
	return nil
}

// planStrategy requires one strategy/key tuple across the whole plan, matching
// the history record's single strategy binding.
func planStrategy(plan Plan) (id, version, keyID string, err error) {
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if id == "" && version == "" && keyID == "" {
				id, version, keyID = column.Strategy.ID, column.Strategy.Version, column.KeyID
				continue
			}
			if column.Strategy.ID != id || column.Strategy.Version != version || column.KeyID != keyID {
				return "", "", "", ErrUnsafe
			}
		}
	}
	if id == "" || version == "" || keyID == "" {
		return "", "", "", ErrUnsafe
	}
	return id, version, keyID, nil
}

func planScopeCount(plan Plan) int {
	count := 0
	for _, table := range plan.Tables {
		count += len(table.Columns)
	}
	return count
}
