package runner

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/testhooks"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const recoveryRunID = "fedcba9876543210fedcba9876543210"

// recoveryFixture is the streaming fixture plus an explicitly installed
// recovery schema, so the run records a manifest and progress log.
func recoveryFixture(t *testing.T, scopes []string, tables map[string][]string) (*Engine, contracts.Plan, string, string, func()) {
	t.Helper()
	requireLinuxStreaming(t)
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		for name, rows := range tables {
			if _, err := database.Exec(`CREATE TABLE ` + name + ` (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
				t.Fatal(err)
			}
			for i, email := range rows {
				var value any = email
				if email == "<nil>" {
					value = nil
				}
				if _, err := database.Exec(`INSERT INTO `+name+` VALUES (?, ?)`, i+1, value); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := contracts.NewValue(contracts.Bytes, "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk")
	var declared []config.Scope
	for _, scope := range scopes {
		table, column, _ := splitScope(scope)
		declared = append(declared, config.Scope{Table: table, Column: column})
	}
	cfg := config.Runtime{
		Source: source, Target: target, Scopes: declared,
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
	targetOpts, err := cfg.DatabaseOptions(false)
	if err != nil {
		t.Fatal(err)
	}
	targetOpts.MaxOpenConns = 2
	opts := db.OutboxOptions{
		DatabaseOptions: targetOpts, Protocol: contracts.StreamingProtocolRevision,
		JournalMode: "delete", MaxReceiptPage: testBudget.MaxReceiptPage,
	}
	if err := db.InstallOutboxSchema(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if err := db.InstallRecoverySchema(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.Plan(context.Background())
	if err != nil {
		_ = engine.Close()
		t.Fatalf("plan: %v", err)
	}
	cleanup := func() { _ = engine.Close() }
	t.Cleanup(cleanup)
	return engine, plan, source, target, cleanup
}

func splitScope(scope string) (string, string, bool) {
	for i := 0; i < len(scope); i++ {
		if scope[i] == '.' {
			return scope[:i], scope[i+1:], true
		}
	}
	return "", "", false
}

func recoveryRequest(plan contracts.Plan, approval contracts.StreamingApproval) StreamingRecoveryRequest {
	return StreamingRecoveryRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
		Budget: testBudget,
	}
}

func applyWithRecovery(t *testing.T, engine *Engine, plan contracts.Plan) contracts.StreamingApplySummary {
	t.Helper()
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
		Budget: testBudget,
	})
	if err != nil {
		t.Fatalf("streaming apply: %v", err)
	}
	if summary.State != contracts.RunStateComplete {
		t.Fatalf("state=%v", summary.State)
	}
	return summary
}

func resume(t *testing.T, engine *Engine, plan contracts.Plan) (contracts.StreamingApplySummary, RecoveryReport, error) {
	t.Helper()
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	return engine.StreamingResume(context.Background(), recoveryRequest(plan, approval))
}

// interruptAfterPages aborts before the (n+1)th page, so exactly n pages were
// committed. It models a process death between two committed batches.
func interruptAfterPages(t *testing.T, pages int) func() {
	t.Helper()
	seen := 0
	restore := testhooks.Set(func(stage string) error {
		if stage == "before_batch" {
			seen++
			if seen > pages {
				panic("simulated process death between batches")
			}
		}
		return nil
	})
	return restore
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected simulated process death")
		}
	}()
	fn()
}

func readTableColumn(t *testing.T, path, table string) map[int]string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`SELECT id, email FROM ` + table + ` ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[int]string{}
	for rows.Next() {
		var id int
		var email sql.NullString
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatal(err)
		}
		result[id] = email.String
	}
	return result
}

func fileDigestOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func TestRecoveryResumeSkipsCommittedBatchesAndIsIdempotent(t *testing.T) {
	rows := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		rows = append(rows, fmt.Sprintf("user%02d@example.invalid", i))
	}
	// A: uninterrupted baseline.
	baselineEngine, baselinePlan, _, baselineTarget, _ := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	applyWithRecovery(t, baselineEngine, baselinePlan)
	baseline := readTableColumn(t, baselineTarget, "people")
	_ = baselineEngine

	// B: interrupted after two committed batches, then resumed.
	engine, plan, source, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	sourceBefore := fileDigestOf(t, source)
	restore := interruptAfterPages(t, 2)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()

	// A new process reopens the same files and resumes.
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, err := reopened.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil {
		t.Fatalf("resume: %v (report %+v)", err, report)
	}
	if summary.State != contracts.RunStateComplete || summary.Audit != contracts.AuditStateConsistent {
		t.Fatalf("summary=%+v report=%+v", summary, report)
	}
	if report.State != db.RecoveryStateComplete || report.Validation != db.ValidationPassed {
		t.Fatalf("report=%+v", report)
	}
	got := readTableColumn(t, target, "people")
	if !reflect.DeepEqual(baseline, got) {
		t.Fatalf("resumed target differs from uninterrupted target")
	}
	if after := fileDigestOf(t, source); after != sourceBefore {
		t.Fatal("source snapshot changed during recovery")
	}
	// Repeated resumes are idempotent: no new receipts and no value change.
	receiptsBefore := receiptCount(t, reopened, recoveryRunID)
	for i := 0; i < 3; i++ {
		summary, _, err = resume(t, reopened, reopenedPlan)
		if err != nil || summary.State != contracts.RunStateComplete {
			t.Fatalf("repeat resume %d: %v %+v", i, err, summary)
		}
	}
	if receiptsAfter := receiptCount(t, reopened, recoveryRunID); receiptsAfter != receiptsBefore {
		t.Fatalf("repeat resume added receipts: %d -> %d", receiptsBefore, receiptsAfter)
	}
	if after := readTableColumn(t, target, "people"); !reflect.DeepEqual(got, after) {
		t.Fatal("repeat resume changed values")
	}
}

func TestRecoveryResumeNullPagesAndMultiTable(t *testing.T) {
	people := []string{"a@example.invalid", "<nil>", "c@example.invalid", "<nil>", "e@example.invalid"}
	contacts := []string{"<nil>", "g@example.invalid", "<nil>", "i@example.invalid"}
	baselineEngine, baselinePlan, _, baselineTarget, _ := recoveryFixture(t,
		[]string{"people.email", "contacts.email"},
		map[string][]string{"people": people, "contacts": contacts})
	applyWithRecovery(t, baselineEngine, baselinePlan)
	baselinePeople := readTableColumn(t, baselineTarget, "people")
	baselineContacts := readTableColumn(t, baselineTarget, "contacts")

	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email", "contacts.email"},
		map[string][]string{"people": people, "contacts": contacts})
	// people has 5 rows in 3 pages (2,2,1), then the table boundary, then the
	// first contacts page. Four pages means interruption during the second table.
	restore := interruptAfterPages(t, 4)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()

	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, err := reopened.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil {
		t.Fatalf("resume: %v %+v", err, report)
	}
	if summary.State != contracts.RunStateComplete || report.TableOrdinal != 2 {
		t.Fatalf("summary=%+v report=%+v", summary, report)
	}
	if !reflect.DeepEqual(baselinePeople, readTableColumn(t, target, "people")) ||
		!reflect.DeepEqual(baselineContacts, readTableColumn(t, target, "contacts")) {
		t.Fatal("multi-table resume differs from uninterrupted result")
	}
}

func TestRecoveryResumeCommitUnknownReconciles(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	engine, plan, _, _, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 1)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	// Model "the commit outcome was ambiguous": the authoritative checkpoint is
	// already advanced, but the manifest says commit_unknown.
	setManifestState(t, engine, recoveryRunID, db.RecoveryStateCommitUnknown)
	cleanup()

	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil {
		t.Fatalf("resume after commit_unknown: %v %+v", err, report)
	}
	if summary.State != contracts.RunStateComplete {
		t.Fatalf("state=%v", summary.State)
	}
	// Exactly two batches: the committed one was not replayed.
	if summary.BatchesCommitted != 2 {
		t.Fatalf("batches=%d, want 2", summary.BatchesCommitted)
	}
}

func TestRecoveryResumeRefusesTamperedTarget(t *testing.T) {
	rows := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		rows = append(rows, fmt.Sprintf("user%02d@example.invalid", i))
	}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 2)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	// External modification in the uncommitted suffix must be a conflict.
	modifyTarget(t, target, `UPDATE people SET email='tampered@example.invalid' WHERE id=8`)
	tampered := readTableColumn(t, target, "people")
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	if _, report, err := resume(t, reopened, reopenedPlan); !errors.Is(err, contracts.ErrConflict) ||
		report.State != db.RecoveryStateConflict {
		t.Fatalf("tampered target: err=%v report=%+v", err, report)
	}
	if after := readTableColumn(t, target, "people"); !reflect.DeepEqual(tampered, after) {
		t.Fatal("conflict path modified business rows")
	}
}

func TestRecoveryResumeRefusesChangedSourceAndSettings(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	engine, plan, source, _, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 1)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()

	// A different batch size produces a different settings digest.
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	reopenedPlan, _ := reopened.Plan(context.Background())
	approval, record := streamingApproval(t, reopenedPlan)
	reopened.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	drifted := recoveryRequest(reopenedPlan, approval)
	drifted.BatchSize = 3
	if _, _, err := reopened.StreamingResume(context.Background(), drifted); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("settings drift: got %v, want ErrConflict", err)
	}
	// A revoked/unverifiable approval is refused before any write.
	reopened.SetStreamingApprovalQuery(&fakeStreamingVerifier{err: errors.New("revoked")})
	if _, _, err := reopened.StreamingResume(context.Background(), recoveryRequest(reopenedPlan, approval)); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("revoked approval: got %v, want ErrUnsafe", err)
	}
	reopened.Close()

	// Changing the source changes the snapshot identity and refuses the plan.
	database, err := sql.Open("sqlite", "file:"+source+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO people VALUES (5, 'late@example.invalid')`); err != nil {
		t.Fatal(err)
	}
	database.Close()
	reopened2, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		// A changed source can also fail at plan build; that is still a refusal.
		return
	}
	defer reopened2.Close()
	reopenedPlan2, _ := reopened2.Plan(context.Background())
	approval2, record2 := streamingApproval(t, reopenedPlan2)
	reopened2.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record2})
	if _, _, err := reopened2.StreamingResume(context.Background(), recoveryRequest(reopenedPlan2, approval2)); err == nil {
		t.Fatal("changed source was accepted")
	}
}

func TestRecoveryResumeRevalidatesAfterInterruptedValidation(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	engine, plan, _, _, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := testhooks.Set(func(stage string) error {
		if stage == "after_validate_before_complete" {
			return errors.New("simulated death after validation")
		}
		return nil
	})
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
		Budget: testBudget,
	})
	restore()
	if err == nil || summary.State == contracts.RunStateComplete {
		t.Fatalf("interrupted validation reported success: %v %+v", err, summary)
	}
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil || summary.State != contracts.RunStateComplete {
		t.Fatalf("resume: %v %+v %+v", err, summary, report)
	}
	if report.Validation != db.ValidationPassed || !report.BusinessComplete {
		t.Fatalf("report=%+v", report)
	}
}

func modifyTarget(t *testing.T, path, statement string) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func setManifestState(t *testing.T, engine *Engine, runID string, state db.RecoveryState) {
	t.Helper()
	manifest, err := readManifestForTest(t, engine, runID)
	if err != nil {
		t.Fatal(err)
	}
	setManifestStateFull(t, engine, runID, manifest.CheckpointSequence, state, false, db.ValidationNone)
}

func setManifestStateFull(t *testing.T, engine *Engine, runID string, expected int64, state db.RecoveryState, business bool, validation db.ValidationState) {
	t.Helper()
	opts, err := engine.streamingOutboxOptions(contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Budget: testBudget,
	})
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := db.OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	recovery, err := outbox.OpenRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := recovery.SetManifestState(context.Background(), runID, expected, state, business, validation); err != nil {
		t.Fatal(err)
	}
}

func readManifestForTest(t *testing.T, engine *Engine, runID string) (db.RecoveryManifest, error) {
	t.Helper()
	opts, err := engine.streamingOutboxOptions(contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Budget: testBudget,
	})
	if err != nil {
		return db.RecoveryManifest{}, err
	}
	outbox, err := db.OpenOutbox(context.Background(), opts)
	if err != nil {
		return db.RecoveryManifest{}, err
	}
	defer outbox.Close()
	recovery, err := outbox.OpenRecovery(context.Background())
	if err != nil {
		return db.RecoveryManifest{}, err
	}
	return recovery.Manifest(context.Background(), runID)
}

func receiptCount(t *testing.T, engine *Engine, runID string) int64 {
	t.Helper()
	opts, err := engine.streamingOutboxOptions(contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Budget: testBudget,
	})
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := db.OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	reader, err := outbox.Reader(runID)
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	for {
		page, err := reader.Page(context.Background(), 8)
		if err != nil {
			t.Fatal(err)
		}
		count += int64(len(page.Receipts))
		if page.Done {
			return count
		}
	}
}

func crashAt(t *testing.T, stage string) func() {
	t.Helper()
	return testhooks.Set(func(current string) error {
		if current == stage {
			panic("simulated process death at " + stage)
		}
		return nil
	})
}

func TestRecoveryResumeCrashBeforeCommitRollsBack(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	baselineEngine, baselinePlan, _, baselineTarget, _ := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	applyWithRecovery(t, baselineEngine, baselinePlan)
	baseline := readTableColumn(t, baselineTarget, "people")

	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := crashAt(t, "before_commit")
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil || summary.State != contracts.RunStateComplete {
		t.Fatalf("resume: %v %+v %+v", err, summary, report)
	}
	if !reflect.DeepEqual(baseline, readTableColumn(t, target, "people")) {
		t.Fatal("after pre-commit crash the result differs")
	}
}

func TestRecoveryResumeCrashAfterCommitBeforeResponse(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	baselineEngine, baselinePlan, _, baselineTarget, _ := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	applyWithRecovery(t, baselineEngine, baselinePlan)
	baseline := readTableColumn(t, baselineTarget, "people")

	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	// The real commit succeeded, then the process died before the caller could
	// observe the response. This is the critical "no double masking" scenario.
	restore := crashAt(t, "after_commit_before_response")
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil || summary.State != contracts.RunStateComplete {
		t.Fatalf("resume: %v %+v %+v", err, summary, report)
	}
	if summary.BatchesCommitted != 2 || summary.RowsCommitted != 4 {
		t.Fatalf("summary=%+v", summary)
	}
	if !reflect.DeepEqual(baseline, readTableColumn(t, target, "people")) {
		t.Fatal("lost-response recovery re-masked or changed values")
	}
}

func TestRecoveryResumeAllNullPages(t *testing.T) {
	// The first page is entirely NULL (a progress-only page), followed by rows
	// that do require masking.
	rows := []string{"<nil>", "<nil>", "c@example.invalid", "d@example.invalid", "e@example.invalid"}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 2)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if err != nil || summary.State != contracts.RunStateComplete {
		t.Fatalf("resume: %v %+v %+v", err, summary, report)
	}
	if summary.BatchesCommitted != 2 || summary.RowsCommitted != 3 {
		t.Fatalf("no-op page was counted as commits: %+v", summary)
	}
	got := readTableColumn(t, target, "people")
	if got[1] != "" || got[2] != "" {
		t.Fatalf("NULL rows changed: %+v", got)
	}
}

func TestRecoveryValidationFailedRunIsNeverComplete(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid"}
	engine, plan, _, _, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	// Stop after all business commits but before validate, so the manifest is
	// not terminal and can be moved to validation_failed (terminal states are
	// immutable within one run).
	restore := testhooks.Set(func(stage string) error {
		if stage == "before_validate" {
			return errors.New("simulated stop before validate")
		}
		return nil
	})
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	if _, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
		Budget: testBudget,
	}); err == nil {
		t.Fatal("expected a pre-validation stop")
	}
	restore()
	manifest, err := readManifestForTest(t, engine, recoveryRunID)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.BusinessComplete {
		t.Fatalf("business not complete before validation: %+v", manifest)
	}
	setManifestStateFull(t, engine, recoveryRunID, manifest.CheckpointSequence, db.RecoveryStateValidationFailed, true, db.ValidationFailed)
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	summary, report, err := resume(t, reopened, reopenedPlan)
	if !errors.Is(err, contracts.ErrIncomplete) || summary.State == contracts.RunStateComplete {
		t.Fatalf("validation-failed resume reported success: %v %+v %+v", err, summary, report)
	}
	if report.State != db.RecoveryStateValidationFailed {
		t.Fatalf("report state=%s", report.State)
	}
}

// TestRecoveryFixedPointReplacementRefusedBeforeWrite documents why the
// prefix rule is equality with the recomputed replacement: the apply path never
// commits a non-NULL replacement equal to the source.
func TestRecoveryFixedPointReplacementRefusedBeforeWrite(t *testing.T) {
	requireLinuxStreaming(t)
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO notes VALUES (1, '')`); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
	cfg := config.Runtime{
		Source: source, Target: target, Table: "notes", Column: "body",
		Strategy: contracts.StrategyRef{ID: "blank", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 1, Workers: 1,
	}
	key, _ := contracts.NewValue(contracts.Bytes, "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk")
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	col := contracts.ColumnPlan{
		Column:   contracts.ColumnRef{TableRef: tableRef("notes"), Column: "body"},
		Type:     contracts.Type{Kind: contracts.Text, Native: "TEXT", Nullable: false},
		Strategy: contracts.StrategyRef{ID: "blank", Version: "1"},
		Scope:    "sqlite/notes/body", NormalizationVersion: "typed-lexical-v1", KeyID: "synthetic",
	}
	empty, _ := contracts.NewValue(contracts.Text, "")
	_, err = engine.maskColumn(context.Background(), empty, col)
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("fixed-point replacement: got %v, want ErrUnsafe", err)
	}
}

func TestRecoveryResumeRefusesWrongKeyBeforeFirstBatch(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := crashAt(t, "before_commit")
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	before := readTableColumn(t, target, "people")
	// Same KeyID, different 32-byte key material. With no committed prefix the
	// value comparison is vacuous, so only the key-check value can catch this.
	wrong, _ := contracts.NewValue(contracts.Bytes, "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	reopened, err := Open(context.Background(), engine.cfg, wrong)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, err := reopened.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, report, err := resume(t, reopened, reopenedPlan); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("wrong key: got %v report=%+v", err, report)
	}
	if after := readTableColumn(t, target, "people"); !reflect.DeepEqual(before, after) {
		t.Fatal("wrong-key resume wrote data")
	}
}

func TestRecoveryResumeDetectsExternalChangeAfterVerification(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid", "e@example.invalid", "f@example.invalid"}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 1)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	tampered := false
	restoreHook := testhooks.Set(func(stage string) error {
		if stage == "before_batch" && !tampered {
			tampered = true
			modifyTarget(t, target, `UPDATE people SET email='late-tamper@example.invalid' WHERE id=6`)
		}
		return nil
	})
	defer restoreHook()
	if _, report, err := resume(t, reopened, reopenedPlan); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("late tamper: got %v report=%+v", err, report)
	}
	if got := readTableColumn(t, target, "people"); got[6] != "late-tamper@example.invalid" {
		t.Fatalf("conflict overwrote the late external change: %+v", got)
	}
}

func TestRecoveryRejectsNonRecoverableStrategy(t *testing.T) {
	plan := contracts.Plan{Tables: []contracts.TablePlan{{Columns: []contracts.ColumnPlan{{
		Strategy: contracts.StrategyRef{ID: "shuffle", Version: "1"},
	}}}}}
	if !errors.Is(validateRecoverablePlan(plan), contracts.ErrUnsupported) {
		t.Fatal("non-recoverable strategy was accepted")
	}
}

func TestRecoveryResumeRejectsManifestAheadOfProgress(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid"}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email"}, map[string][]string{"people": rows})
	restore := interruptAfterPages(t, 1)
	mustPanic(t, func() {
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		_, _ = engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: recoveryRunID, BatchSize: 2, ReceiptPageSize: 8,
			Budget: testBudget,
		})
	})
	restore()
	cleanup()
	// Manifest restored ahead of the authoritative log is tamper evidence.
	modifyTarget(t, target, `UPDATE maskriver_recovery_manifest SET checkpoint_sequence = checkpoint_sequence + 1 WHERE run_id='`+recoveryRunID+`'`)
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	if _, report, err := resume(t, reopened, reopenedPlan); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("manifest ahead of log: got %v report=%+v", err, report)
	}
}

func TestRecoveryRejectsBusinessCompleteBeforeLastTable(t *testing.T) {
	people := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid"}
	contacts := []string{"d@example.invalid", "e@example.invalid"}
	engine, plan, _, target, cleanup := recoveryFixture(t,
		[]string{"people.email", "contacts.email"},
		map[string][]string{"people": people, "contacts": contacts})
	applyWithRecovery(t, engine, plan)
	cleanup()
	// A tampered final boundary placed at table ordinal 1 (there are two tables)
	// must not authorize skipping the second table.
	modifyTarget(t, target, `UPDATE maskriver_recovery_progress SET table_ordinal = 1 WHERE run_id = '`+recoveryRunID+`' AND business_complete = 1`)
	reopened, err := Open(context.Background(), engine.cfg, engine.key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedPlan, _ := reopened.Plan(context.Background())
	if _, report, err := resume(t, reopened, reopenedPlan); !errors.Is(err, contracts.ErrConflict) ||
		report.State != db.RecoveryStateConflict {
		t.Fatalf("early business_complete: err=%v report=%+v", err, report)
	}
}

func TestRecoveryBusinessCompleteRequiresFinalTableOrdinal(t *testing.T) {
	if !businessCompleteValid(db.RecoveryProgress{BusinessComplete: true, TableOrdinal: 2}, 2) {
		t.Fatal("final boundary rejected")
	}
	if businessCompleteValid(db.RecoveryProgress{BusinessComplete: true, TableOrdinal: 1}, 2) {
		t.Fatal("early business_complete accepted")
	}
	if !businessCompleteValid(db.RecoveryProgress{BusinessComplete: false, TableOrdinal: 1}, 2) {
		t.Fatal("incomplete progress rejected")
	}
}
