package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// streamingGoldenPlanID is pinned from the unchanged m1a plan-ID formula
// sha256(Revision + SourceSnapshotID + TargetID + SchemaDigest + ConfigDigest).
// It must not be regenerated from production code: that would make the
// compatibility test vacuous.
const streamingGoldenPlanID = "31202fec390c215f5b7e7d8308e184a1488e13857fcb650ba683faf0c0ffa272"

func streamingTestNow() time.Time {
	return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
}

func streamingTestPlan() Plan {
	ref := TableRef{Database: "app", Schema: "main", Table: "people"}
	return Plan{
		Revision:         APIRevision,
		ID:               streamingGoldenPlanID,
		SourceSnapshotID: "snapshot-synthetic-001",
		TargetID:         "target-synthetic-001",
		SchemaDigest:     strings.Repeat("b", 64),
		ConfigDigest:     strings.Repeat("c", 64),
		ScanComplete:     true,
		Tables: []TablePlan{{
			Table:      ref,
			PrimaryKey: []string{"id"},
			Columns: []ColumnPlan{{
				Column:               ColumnRef{TableRef: ref, Column: "email"},
				Type:                 Type{Kind: Text},
				Strategy:             StrategyRef{ID: "email", Version: "v1"},
				Scope:                "people.email",
				NormalizationVersion: "typed-lexical-v1",
				KeyID:                "key-1",
			}},
		}},
	}
}

func streamingTestApproval(plan Plan) StreamingApproval {
	now := streamingTestNow()
	return StreamingApproval{
		BindingRevision:  ApprovalBindingRevision,
		PlanID:           plan.ID,
		SourceSnapshotID: plan.SourceSnapshotID,
		TargetID:         plan.TargetID,
		SchemaDigest:     plan.SchemaDigest,
		ConfigDigest:     plan.ConfigDigest,
		StrategyID:       "email",
		StrategyVersion:  "v1",
		KeyID:            "key-1",
		Scopes:           []StreamingScope{{Table: plan.Tables[0].Table, Column: "email"}},
		IssuedAt:         now.Add(-time.Hour),
		ExpiresAt:        now.Add(time.Hour),
	}
}

func streamingTestBudget() ResourceBudget {
	return ResourceBudget{
		MaxBatchSize:        64,
		MaxWorkers:          4,
		MaxRowBytes:         64 << 10,
		MaxColumns:          32,
		MaxTables:           8,
		MaxHistoryEntries:   10_000,
		MaxHistoryNameBytes: 4 << 20,
		MaxReceiptPage:      100,
		MaxIssueCounts:      16,
		MaxQueue:            0,
		MaxProcessBytes:     1 << 30,
		MaxDiskBytes:        1 << 30,
	}
}

func streamingTestOutbox() OutboxCapabilities {
	return OutboxCapabilities{
		Preprovisioned:        true,
		SameDatabase:          true,
		Transactional:         true,
		SupportsCommitUnknown: true,
		SchemaVersion:         "outbox-v1",
		ProtocolVersion:       StreamingProtocolRevision,
		MaxReceiptPage:        100,
	}
}

func streamingTestRequest() StreamingApplyRequest {
	plan := streamingTestPlan()
	return StreamingApplyRequest{
		Protocol:        StreamingProtocolRevision,
		Plan:            plan,
		Approval:        streamingTestApproval(plan),
		ExplicitApply:   true,
		RunID:           "0123456789abcdef0123456789abcdef",
		BatchSize:       64,
		ReceiptPageSize: 50,
		Budget:          streamingTestBudget(),
		Outbox:          streamingTestOutbox(),
	}
}

func TestStreamingRequestValid(t *testing.T) {
	if err := streamingTestRequest().Validate(streamingTestNow()); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestStreamingRequestRejectsUnsafeOrUnsupported(t *testing.T) {
	cases := []struct {
		name string
		edit func(*StreamingApplyRequest)
		want error
	}{
		{"unknown protocol", func(r *StreamingApplyRequest) { r.Protocol = "m9-future" }, ErrUnsupported},
		{"missing explicit apply", func(r *StreamingApplyRequest) { r.ExplicitApply = false }, ErrUnsafe},
		{"short run id", func(r *StreamingApplyRequest) { r.RunID = "abcd" }, ErrInvalid},
		{"uppercase run id", func(r *StreamingApplyRequest) { r.RunID = strings.ToUpper(r.RunID) }, ErrInvalid},
		{"non-hex run id", func(r *StreamingApplyRequest) { r.RunID = strings.Repeat("z", 32) }, ErrInvalid},
		{"batch size above budget", func(r *StreamingApplyRequest) { r.BatchSize = r.Budget.MaxBatchSize + 1 }, ErrInvalid},
		{"zero batch size", func(r *StreamingApplyRequest) { r.BatchSize = 0 }, ErrInvalid},
		{"page above budget", func(r *StreamingApplyRequest) { r.ReceiptPageSize = r.Budget.MaxReceiptPage + 1 }, ErrInvalid},
		{"page above outbox capability", func(r *StreamingApplyRequest) {
			r.ReceiptPageSize = 100
			r.Outbox.MaxReceiptPage = 50
		}, ErrInvalid},
		{"budget not supplied", func(r *StreamingApplyRequest) { r.Budget = ResourceBudget{} }, ErrInvalid},
		{"negative queue", func(r *StreamingApplyRequest) { r.Budget.MaxQueue = -1 }, ErrInvalid},
		{"plan over table cap", func(r *StreamingApplyRequest) { r.Budget.MaxTables = 0 }, ErrInvalid},
		{"plan over column cap", func(r *StreamingApplyRequest) { r.Budget.MaxColumns = 0 }, ErrInvalid},
		{"outbox not preprovisioned", func(r *StreamingApplyRequest) { r.Outbox.Preprovisioned = false }, ErrUnsupported},
		{"outbox other database", func(r *StreamingApplyRequest) { r.Outbox.SameDatabase = false }, ErrUnsupported},
		{"outbox non transactional", func(r *StreamingApplyRequest) { r.Outbox.Transactional = false }, ErrUnsupported},
		{"outbox cannot classify unknown", func(r *StreamingApplyRequest) { r.Outbox.SupportsCommitUnknown = false }, ErrUnsupported},
		{"outbox schema missing", func(r *StreamingApplyRequest) { r.Outbox.SchemaVersion = "" }, ErrUnsupported},
		{"outbox protocol mismatch", func(r *StreamingApplyRequest) {
			r.Outbox.ProtocolVersion = "m2-stream-v2"
		}, ErrUnsupported},
		{"approval plan id mismatch", func(r *StreamingApplyRequest) { r.Approval.PlanID = "other" }, ErrUnsafe},
		{"approval snapshot mismatch", func(r *StreamingApplyRequest) { r.Approval.SourceSnapshotID = "other" }, ErrUnsafe},
		{"approval target mismatch", func(r *StreamingApplyRequest) { r.Approval.TargetID = "other" }, ErrUnsafe},
		{"approval schema digest mismatch", func(r *StreamingApplyRequest) { r.Approval.SchemaDigest = "d" }, ErrUnsafe},
		{"approval config digest mismatch", func(r *StreamingApplyRequest) { r.Approval.ConfigDigest = "d" }, ErrUnsafe},
		{"approval strategy mismatch", func(r *StreamingApplyRequest) { r.Approval.StrategyVersion = "v2" }, ErrUnsafe},
		{"approval key mismatch", func(r *StreamingApplyRequest) { r.Approval.KeyID = "key-2" }, ErrUnsafe},
		{"approval revoked", func(r *StreamingApplyRequest) { r.Approval.Revoked = true }, ErrUnsafe},
		{"approval separate scope subset", func(r *StreamingApplyRequest) { r.Approval.Scopes = nil }, ErrUnsafe},
		{"approval reordered scope", func(r *StreamingApplyRequest) {
			ref := r.Plan.Tables[0].Table
			r.Plan.Tables[0].Columns = append(r.Plan.Tables[0].Columns, ColumnPlan{
				Column:               ColumnRef{TableRef: ref, Column: "phone"},
				Type:                 Type{Kind: Text},
				Strategy:             StrategyRef{ID: "email", Version: "v1"},
				Scope:                "people.phone",
				NormalizationVersion: "typed-lexical-v1",
				KeyID:                "key-1",
			})
			r.Approval.Scopes = []StreamingScope{
				{Table: ref, Column: "phone"},
				{Table: ref, Column: "email"},
			}
		}, ErrUnsafe},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := streamingTestRequest()
			tt.edit(&req)
			err := req.Validate(streamingTestNow())
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
		})
	}
}

func TestStreamingApprovalWindow(t *testing.T) {
	plan := streamingTestPlan()
	now := streamingTestNow()

	expired := streamingTestApproval(plan)
	expired.ExpiresAt = now
	if err := ValidateStreamingApproval(plan, expired, now); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("expired approval accepted: %v", err)
	}

	future := streamingTestApproval(plan)
	future.IssuedAt = now.Add(time.Minute)
	if err := ValidateStreamingApproval(plan, future, now); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("future approval accepted: %v", err)
	}

	badWindow := streamingTestApproval(plan)
	badWindow.ExpiresAt = badWindow.IssuedAt
	if err := ValidateStreamingApproval(plan, badWindow, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("inverted window accepted: %v", err)
	}

	// A legacy m1a approval has no streaming binding and must not be upgraded.
	legacy := streamingTestApproval(plan)
	legacy.BindingRevision = ""
	if err := ValidateStreamingApproval(plan, legacy, now); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("legacy approval accepted: %v", err)
	}
}

func TestStreamingMixedStrategyPlanRejected(t *testing.T) {
	plan := streamingTestPlan()
	plan.Tables[0].Columns = append(plan.Tables[0].Columns, ColumnPlan{
		Column:               ColumnRef{TableRef: plan.Tables[0].Table, Column: "phone"},
		Type:                 Type{Kind: Text},
		Strategy:             StrategyRef{ID: "phone", Version: "v1"},
		Scope:                "people.phone",
		NormalizationVersion: "typed-lexical-v1",
		KeyID:                "key-1",
	})
	approval := streamingTestApproval(plan)
	approval.Scopes = append(approval.Scopes, StreamingScope{Table: plan.Tables[0].Table, Column: "phone"})
	if err := ValidateStreamingApproval(plan, approval, streamingTestNow()); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("mixed-strategy plan accepted: %v", err)
	}
}

func TestStreamingReceiptPage(t *testing.T) {
	ref := TableRef{Database: "app", Schema: "main", Table: "people"}
	receipt := func(ordinal int64) StreamingBatchReceipt {
		return StreamingBatchReceipt{Table: ref, BatchID: "batch-" + string(rune('a'+ordinal)), Ordinal: ordinal, Rows: 10, Outcome: BatchCommitted}
	}

	valid := ReceiptPage{Receipts: []StreamingBatchReceipt{receipt(0), receipt(1)}, NextOrdinal: 2}
	if err := ValidateReceiptPage(valid, 0, 2); err != nil {
		t.Fatalf("valid page rejected: %v", err)
	}

	done := ReceiptPage{Receipts: []StreamingBatchReceipt{receipt(0)}, NextOrdinal: 1, Done: true}
	if err := ValidateReceiptPage(done, 0, 10); err != nil {
		t.Fatalf("done page rejected: %v", err)
	}

	emptyDone := ReceiptPage{Done: true}
	if err := ValidateReceiptPage(emptyDone, 5, 10); err != nil {
		t.Fatalf("empty done page rejected: %v", err)
	}

	cases := []struct {
		name     string
		page     ReceiptPage
		expected int64
		limit    int
		want     error
	}{
		{"limit zero", valid, 0, 0, ErrInvalid},
		{"page over limit", valid, 0, 1, ErrInvalid},
		{"out of order", ReceiptPage{Receipts: []StreamingBatchReceipt{receipt(1), receipt(0)}, NextOrdinal: 2}, 0, 2, ErrConflict},
		{"skipped ordinal", ReceiptPage{Receipts: []StreamingBatchReceipt{receipt(2)}, NextOrdinal: 3}, 0, 2, ErrConflict},
		{"wrong expected", valid, 1, 2, ErrConflict},
		{"empty not done", ReceiptPage{}, 0, 2, ErrInvalid},
		{"next ordinal wrong", ReceiptPage{Receipts: []StreamingBatchReceipt{receipt(0)}, NextOrdinal: 5}, 0, 2, ErrConflict},
		{"unknown outcome", ReceiptPage{Receipts: []StreamingBatchReceipt{{Table: ref, BatchID: "b", Rows: 1, Outcome: BatchOutcomeUnknown}}, NextOrdinal: 1}, 0, 2, ErrInvalid},
		{"negative rows", ReceiptPage{Receipts: []StreamingBatchReceipt{{Table: ref, BatchID: "b", Rows: -1, Outcome: BatchCommitted}}, NextOrdinal: 1}, 0, 2, ErrInvalid},
		{"empty batch id", ReceiptPage{Receipts: []StreamingBatchReceipt{{Table: ref, Rows: 1, Outcome: BatchCommitted}}, NextOrdinal: 1}, 0, 2, ErrInvalid},
		{"missing table", ReceiptPage{Receipts: []StreamingBatchReceipt{{BatchID: "b", Rows: 1, Outcome: BatchCommitted}}, NextOrdinal: 1}, 0, 2, ErrInvalid},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateReceiptPage(tt.page, tt.expected, tt.limit); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
		})
	}
}

func TestStreamingSummaryRejectsFalseComplete(t *testing.T) {
	budget := streamingTestBudget()
	valid := StreamingApplySummary{
		Protocol:         StreamingProtocolRevision,
		PlanID:           streamingGoldenPlanID,
		RunID:            "0123456789abcdef0123456789abcdef",
		State:            RunStateComplete,
		Audit:            AuditStateConsistent,
		BatchesTotal:     10,
		BatchesCommitted: 10,
		RowsCommitted:    100,
		ReceiptPageSize:  50,
	}
	if err := valid.Validate(budget); err != nil {
		t.Fatalf("valid summary rejected: %v", err)
	}

	cases := []struct {
		name string
		edit func(*StreamingApplySummary)
		want error
	}{
		{"complete with incomplete audit", func(s *StreamingApplySummary) { s.Audit = AuditStateIncomplete }, ErrIncomplete},
		{"complete with unknown batch", func(s *StreamingApplySummary) { s.BatchesUnknown = 1 }, ErrIncomplete},
		{"complete with failed batch", func(s *StreamingApplySummary) { s.BatchesFailed = 1 }, ErrIncomplete},
		{"complete with unaccounted batches", func(s *StreamingApplySummary) { s.BatchesTotal = 11 }, ErrIncomplete},
		{"consistent audit with unknown batch", func(s *StreamingApplySummary) { s.BatchesUnknown = 1 }, ErrIncomplete},
		{"audit incomplete state with consistent audit", func(s *StreamingApplySummary) {
			s.State = RunStateAuditIncomplete
			s.Audit = AuditStateConsistent
		}, ErrIncomplete},
		{"commit unknown state without unknown batch", func(s *StreamingApplySummary) {
			s.State = RunStateCommitUnknown
			s.Audit = AuditStateIncomplete
		}, ErrIncomplete},
		{"commit unknown state with consistent audit", func(s *StreamingApplySummary) {
			s.State = RunStateCommitUnknown
			s.BatchesUnknown = 1
			s.Audit = AuditStateConsistent
		}, ErrIncomplete},
		{"undefined audit state", func(s *StreamingApplySummary) { s.Audit = AuditStateUnknown }, ErrInvalid},
		{"outcomes exceed total", func(s *StreamingApplySummary) { s.BatchesTotal = 1 }, ErrIncomplete},
		{"unknown run state", func(s *StreamingApplySummary) { s.State = RunStateUnknown }, ErrInvalid},
		{"wrong protocol", func(s *StreamingApplySummary) { s.Protocol = "m9" }, ErrInvalid},
		{"bad run id", func(s *StreamingApplySummary) { s.RunID = "x" }, ErrInvalid},
		{"negative rows", func(s *StreamingApplySummary) { s.RowsCommitted = -1 }, ErrInvalid},
		{"negative total", func(s *StreamingApplySummary) { s.BatchesTotal = -1 }, ErrInvalid},
		{"zero page size", func(s *StreamingApplySummary) { s.ReceiptPageSize = 0 }, ErrInvalid},
		{"page above budget", func(s *StreamingApplySummary) { s.ReceiptPageSize = budget.MaxReceiptPage + 1 }, ErrInvalid},
		{"issue count over budget", func(s *StreamingApplySummary) {
			s.IssueCounts = make([]IssueCount, budget.MaxIssueCounts+1)
			for i := range s.IssueCounts {
				s.IssueCounts[i] = IssueCount{Status: Warning, Code: PartialCoverage, Count: 1}
			}
		}, ErrInvalid},
		{"issue without code", func(s *StreamingApplySummary) { s.IssueCounts = []IssueCount{{Status: Warning}} }, ErrInvalid},
		{"issue with zero status", func(s *StreamingApplySummary) { s.IssueCounts = []IssueCount{{Code: PartialCoverage, Count: 1}} }, ErrInvalid},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			summary := valid
			tt.edit(&summary)
			if err := summary.Validate(budget); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
		})
	}

	running := valid
	running.State = RunStateRunning
	running.Audit = AuditStateUnknown
	if err := running.Validate(budget); err != nil {
		t.Fatalf("running summary rejected: %v", err)
	}
	cancelled := valid
	cancelled.State = RunStateIncomplete
	cancelled.BatchesTotal = 12
	cancelled.BatchesCommitted = 10
	if err := cancelled.Validate(budget); err != nil {
		t.Fatalf("incomplete summary rejected: %v", err)
	}
}

func TestClassifyBatch(t *testing.T) {
	cases := []struct {
		name      string
		attempted bool
		commitErr error
		receipt   error
		want      BatchOutcome
	}{
		{"never attempted", false, nil, nil, BatchFailedBeforeCommit},
		{"never attempted with error", false, ErrDatabase, ErrClosed, BatchFailedBeforeCommit},
		{"confirmed", true, nil, nil, BatchCommitted},
		{"confirmed without receipt", true, nil, ErrClosed, BatchAuditIncomplete},
		{"unknown", true, ErrCommitUnknown, nil, BatchCommitUnknown},
		{"unknown without receipt", true, ErrCommitUnknown, ErrClosed, BatchCommitUnknown},
		{"attempted known failure", true, ErrConflict, nil, BatchCommitFailed},
		{"attempted known failure without receipt", true, ErrConflict, ErrClosed, BatchCommitFailed},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyBatch(tt.attempted, tt.commitErr, tt.receipt); got != tt.want {
				t.Fatalf("ClassifyBatch(%v,%v,%v)=%v want %v", tt.attempted, tt.commitErr, tt.receipt, got, tt.want)
			}
		})
	}
	if got := ClassifyCommitResult(nil); got != CommitSucceeded {
		t.Fatalf("nil commit=%v", got)
	}
	if got := ClassifyCommitResult(ErrCommitUnknown); got != CommitOutcomeUnknown {
		t.Fatalf("unknown commit=%v", got)
	}
	if got := ClassifyCommitResult(ErrDatabase); got != CommitFailed {
		t.Fatalf("failed commit=%v", got)
	}
	if got := ClassifyCommitResult(fmt.Errorf("commit: %w", ErrCommitUnknown)); got != CommitOutcomeUnknown {
		t.Fatalf("wrapped unknown commit=%v", got)
	}
	if CommitSucceeded == CommitClassification(0) {
		t.Fatal("zero commit classification must not mean success")
	}
}

func TestStreamingTypesCarryNoValues(t *testing.T) {
	summary := StreamingApplySummary{
		Protocol:         StreamingProtocolRevision,
		PlanID:           streamingGoldenPlanID,
		RunID:            "0123456789abcdef0123456789abcdef",
		State:            RunStateIncomplete,
		Audit:            AuditStateIncomplete,
		BatchesTotal:     1,
		BatchesCommitted: 1,
		ReceiptPageSize:  10,
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"Receipts", "Values", "Replacement", "Key ", "DSN"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("summary exposes %q: %s", forbidden, raw)
		}
	}
	if reflect.TypeOf(StreamingBatchReceipt{}).NumField() != 5 {
		t.Fatal("receipt field set changed; review whether it can carry sensitive data")
	}
}

func TestStreamingBudgetRequiresExplicitValues(t *testing.T) {
	budget := streamingTestBudget()
	if err := budget.Validate(); err != nil {
		t.Fatalf("valid budget rejected: %v", err)
	}
	fields := []func(*ResourceBudget){
		func(b *ResourceBudget) { b.MaxBatchSize = 0 },
		func(b *ResourceBudget) { b.MaxWorkers = 0 },
		func(b *ResourceBudget) { b.MaxRowBytes = 0 },
		func(b *ResourceBudget) { b.MaxColumns = 0 },
		func(b *ResourceBudget) { b.MaxTables = 0 },
		func(b *ResourceBudget) { b.MaxHistoryEntries = 0 },
		func(b *ResourceBudget) { b.MaxHistoryNameBytes = 0 },
		func(b *ResourceBudget) { b.MaxReceiptPage = 0 },
		func(b *ResourceBudget) { b.MaxIssueCounts = 0 },
		func(b *ResourceBudget) { b.MaxProcessBytes = 0 },
		func(b *ResourceBudget) { b.MaxDiskBytes = 0 },
	}
	for i, mutate := range fields {
		candidate := streamingTestBudget()
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("field %d zero accepted: %v", i, err)
		}
	}
	queue := streamingTestBudget()
	queue.MaxQueue = 0
	if err := queue.Validate(); err != nil {
		t.Fatalf("synchronous queue rejected: %v", err)
	}
}

func TestOutboxCapabilitiesSupports(t *testing.T) {
	caps := streamingTestOutbox()
	if !caps.Supports(StreamingProtocolRevision) {
		t.Fatal("valid capability not supported")
	}
	if caps.Supports("m2-stream-v2") {
		t.Fatal("unknown protocol supported")
	}
	legacy := caps
	legacy.ProtocolVersion = APIRevision
	if legacy.Supports(StreamingProtocolRevision) {
		t.Fatal("legacy protocol advertised as streaming")
	}
}

func FuzzStreamingRequestValidate(f *testing.F) {
	f.Add(StreamingProtocolRevision, "0123456789abcdef0123456789abcdef", 1, 1)
	f.Add("", "", 0, 0)
	f.Add("m2-stream-v1", strings.Repeat("f", 32), 1<<20, -1)
	f.Fuzz(func(t *testing.T, protocol, runID string, batchSize, pageSize int) {
		req := streamingTestRequest()
		req.Protocol = protocol
		req.RunID = runID
		req.BatchSize = batchSize
		req.ReceiptPageSize = pageSize
		_ = req.Validate(streamingTestNow())
	})
}

// --- legacy compatibility ---

func TestAPIRevisionFrozen(t *testing.T) {
	if APIRevision != "m1a-v1" {
		t.Fatalf("APIRevision=%q", APIRevision)
	}
	if StreamingProtocolRevision == APIRevision {
		t.Fatal("streaming revision must be independent")
	}
}

func TestLegacyPlanIDFormulaUnchanged(t *testing.T) {
	plan := streamingTestPlan()
	sum := sha256.Sum256([]byte(plan.Revision + plan.SourceSnapshotID + plan.TargetID + plan.SchemaDigest + plan.ConfigDigest))
	want := hex.EncodeToString(sum[:])
	if plan.ID != want {
		t.Fatalf("fixture ID=%s does not match formula %s", plan.ID, want)
	}
	if plan.ID != streamingGoldenPlanID {
		t.Fatalf("plan ID changed: %s", plan.ID)
	}
	if err := plan.ValidateShape(); err != nil {
		t.Fatalf("legacy plan rejected: %v", err)
	}
	if err := ValidatePlanIdentity(plan); err != nil {
		t.Fatalf("legacy identity rejected: %v", err)
	}
}

func TestPlanIdentityTamperRejected(t *testing.T) {
	plan := streamingTestPlan()
	plan.SchemaDigest = strings.Repeat("d", 64)
	if err := ValidatePlanIdentity(plan); !errors.Is(err, ErrConflict) {
		t.Fatalf("tampered digest with stale ID accepted: %v", err)
	}
	if err := ValidateStreamingApproval(plan, streamingTestApproval(plan), streamingTestNow()); !errors.Is(err, ErrConflict) {
		t.Fatalf("approval validated against tampered identity: %v", err)
	}
	// Limitation is explicit: per-column Scope/NormalizationVersion are not part
	// of the m1a plan-ID formula and require the canonical plan-content check.
	if err := plan.ValidateShape(); err != nil {
		t.Fatalf("shape check must still pass: %v", err)
	}
}

func TestLegacyApplyShapeUnchanged(t *testing.T) {
	want := map[string]string{
		"PlanID":               "string",
		"Committed":            "[]contracts.BatchReceipt",
		"CommitOutcomeUnknown": "bool",
		"Complete":             "bool",
		"Issues":               "[]contracts.Issue",
	}
	typ := reflect.TypeOf(ApplyReport{})
	if typ.NumField() != len(want) {
		t.Fatalf("ApplyReport field count=%d", typ.NumField())
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if want[field.Name] != field.Type.String() {
			t.Fatalf("field %s type=%s want %s", field.Name, field.Type, want[field.Name])
		}
	}
	receipt := reflect.TypeOf(BatchReceipt{})
	if receipt.NumField() != 3 {
		t.Fatalf("BatchReceipt field count=%d", receipt.NumField())
	}
}

func TestStreamingRevisionDoesNotChangePlanShape(t *testing.T) {
	plan := streamingTestPlan()
	plan.Revision = StreamingProtocolRevision
	if err := plan.ValidateShape(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("plan accepted streaming revision: %v", err)
	}
}
