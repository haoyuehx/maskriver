package runner

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// streamingApproval builds the verified-looking record a store would return and
// the matching request approval. The real signature path is covered by the
// history tests and the CLI E2E; here the runner's own binding checks are
// exercised.
func streamingApproval(t *testing.T, plan contracts.Plan) (contracts.StreamingApproval, history.StreamingRecord) {
	t.Helper()
	digest, err := history.CanonicalPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	record := history.StreamingRecord{
		Version: history.StreamingRecordVersion, Protocol: contracts.StreamingProtocolRevision,
		BindingRevision: contracts.ApprovalBindingRevision, PlanID: plan.ID,
		SourceSnapshotID: plan.SourceSnapshotID, TargetID: plan.TargetID,
		SchemaDigest: plan.SchemaDigest, ConfigDigest: plan.ConfigDigest,
		PlanContentDigest: digest, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
	}
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if record.StrategyID == "" {
				record.StrategyID = column.Strategy.ID
				record.StrategyVersion = column.Strategy.Version
				record.KeyID = column.KeyID
			}
			record.Scopes = append(record.Scopes, history.StreamingScope{
				Table: table.Table, Column: column.Column.Column, Scope: column.Scope,
				NormalizationVersion: column.NormalizationVersion, Type: column.Type,
			})
		}
	}
	return record.StreamingApproval(), record
}

type fakeStreamingVerifier struct {
	record history.StreamingRecord
	err    error
	calls  int
}

func (f *fakeStreamingVerifier) VerifyStreaming(_ context.Context, _ contracts.Plan, _ contracts.StreamingApproval) (history.StreamingRecord, error) {
	f.calls++
	if f.err != nil {
		return history.StreamingRecord{}, f.err
	}
	return f.record, nil
}

var testBudget = contracts.ResourceBudget{
	MaxBatchSize: 256, MaxWorkers: 16, MaxRowBytes: 1 << 20, MaxColumns: 100,
	MaxTables: 10, MaxHistoryEntries: 1000, MaxHistoryNameBytes: 4096,
	MaxReceiptPage: 64, MaxIssueCounts: 16, MaxQueue: 0,
	MaxProcessBytes: 2 << 30, MaxDiskBytes: 8 << 30,
}

// requireLinuxStreaming skips when the SQLite outbox is explicitly unsupported.
// The unsupported refusal itself is covered by internal/db unit tests.
func requireLinuxStreaming(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("streaming outbox is Linux-only; the refusal is covered by db tests")
	}
}

func streamingFixture(t *testing.T, rows []string) (*Engine, contracts.Plan, string, func()) {
	t.Helper()
	requireLinuxStreaming(t)
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	cfg, key := streamingFixtureFiles(t, rows, source, target)
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
		t.Fatalf("install outbox: %v", err)
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
	return engine, plan, target, cleanup
}

// streamingFixtureFiles builds the synthetic pair used by the streaming tests.
func streamingFixtureFiles(t *testing.T, rows []string, source, target string) (config.Runtime, contracts.Value) {
	t.Helper()
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
			t.Fatal(err)
		}
		for i, email := range rows {
			if _, err := database.Exec(`INSERT INTO people VALUES (?, ?)`, i+1, email); err != nil {
				t.Fatal(err)
			}
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := contracts.NewValue(contracts.Bytes, "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk")
	return config.Runtime{
		Source: source, Target: target, Table: "people", Column: "email",
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 2, Workers: 1,
	}, key
}

func TestStreamingApplyCommitsEveryBatch(t *testing.T) {
	rows := []string{"a@example.invalid", "b@example.invalid", "c@example.invalid", "d@example.invalid", "e@example.invalid"}
	engine, plan, _, _ := streamingFixture(t, rows)
	approval, record := streamingApproval(t, plan)
	verifier := &fakeStreamingVerifier{record: record}
	engine.SetStreamingApprovalQuery(verifier)

	summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: "0123456789abcdef0123456789abcdef",
		BatchSize: 2, ReceiptPageSize: 8, Budget: testBudget,
	})
	if err != nil {
		t.Fatalf("streaming apply: %v", err)
	}
	if summary.State != contracts.RunStateComplete || summary.Audit != contracts.AuditStateConsistent {
		t.Fatalf("summary state=%v audit=%v", summary.State, summary.Audit)
	}
	if summary.BatchesCommitted != 3 || summary.BatchesTotal != 3 || summary.RowsCommitted != 5 {
		t.Fatalf("summary=%+v", summary)
	}
	if verifier.calls != 1 {
		t.Fatalf("approval verifier calls=%d", verifier.calls)
	}
	if len(summary.IssueCounts) != 0 {
		t.Fatalf("unexpected issue counts: %+v", summary.IssueCounts)
	}
	// The outbox must hold exactly the committed batches, read back in bounded
	// pages (never from an in-memory list).
	opts, err := engine.streamingOutboxOptions(contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Budget: testBudget, ReceiptPageSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := db.OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()
	reader, err := outbox.Reader(summary.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var receipts int64
	var ordinal int64
	for {
		page, err := reader.Page(context.Background(), 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, receipt := range page.Receipts {
			if receipt.Ordinal != ordinal {
				t.Fatalf("receipt ordinal=%d want %d", receipt.Ordinal, ordinal)
			}
			ordinal++
			receipts++
		}
		if page.Done {
			break
		}
	}
	if receipts != 3 {
		t.Fatalf("receipts=%d", receipts)
	}
	// Source unchanged; target masked; validation already ran inside the call.
	if before, after := readEmailColumn(t, engine.cfg.Source, "people"),
		readEmailColumn(t, engine.cfg.Target, "people"); reflect.DeepEqual(before, after) {
		t.Fatal("target was not masked")
	}
}

func TestStreamingApplyWorkersAgree(t *testing.T) {
	rows := make([]string, 0, 16)
	for i := 0; i < 16; i++ {
		rows = append(rows, fmt.Sprintf("user%02d@example.invalid", i))
	}
	var baseline map[int]string
	for _, workers := range []int{1, 2, 4, 8, 16} {
		rowsCopy := append([]string(nil), rows...)
		engine, plan, target, _ := streamingFixture(t, rowsCopy)
		engine.cfg.Workers = workers
		approval, record := streamingApproval(t, plan)
		engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
		summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
			Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
			ExplicitApply: true, RunID: "0123456789abcdef0123456789abcdef",
			BatchSize: 3, ReceiptPageSize: 8, Budget: testBudget,
		})
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}
		if summary.State != contracts.RunStateComplete {
			t.Fatalf("workers=%d state=%v", workers, summary.State)
		}
		got := readEmailColumn(t, target, "people")
		if baseline == nil {
			baseline = got
			continue
		}
		if !reflect.DeepEqual(baseline, got) {
			t.Fatalf("workers=%d produced different target values", workers)
		}
	}
}

func TestStreamingApplyRefusesUnverifiedApproval(t *testing.T) {
	engine, plan, target, _ := streamingFixture(t, []string{"a@example.invalid", "b@example.invalid"})
	approval, record := streamingApproval(t, plan)
	before := readEmailColumn(t, target, "people")

	// No verifier at all.
	if _, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: "0123456789abcdef0123456789abcdef",
		BatchSize: 2, ReceiptPageSize: 8, Budget: testBudget,
	}); err == nil {
		t.Fatal("nil verifier authorized a run")
	}
	// Legacy binding revision never authorizes m2.
	legacy := approval
	legacy.BindingRevision = "m1a-v1"
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	if _, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: legacy,
		ExplicitApply: true, RunID: "0123456789abcdef0123456789abcdef",
		BatchSize: 2, ReceiptPageSize: 8, Budget: testBudget,
	}); err == nil {
		t.Fatal("legacy binding authorized streaming")
	}
	// Plan content drift under the same Plan.ID is refused by the canonical
	// digest check, not only by Plan.ID.
	drifted := plan
	drifted.Tables = append([]contracts.TablePlan(nil), plan.Tables...)
	drifted.Tables[0].Columns = append([]contracts.ColumnPlan(nil), plan.Tables[0].Columns...)
	drifted.Tables[0].Columns[0].Scope = "sqlite/people/drifted"
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	if _, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: drifted, Approval: approval,
		ExplicitApply: true, RunID: "0123456789abcdef0123456789abcdef",
		BatchSize: 2, ReceiptPageSize: 8, Budget: testBudget,
	}); err == nil {
		t.Fatal("content drift authorized streaming")
	}
	if after := readEmailColumn(t, target, "people"); !reflect.DeepEqual(before, after) {
		t.Fatal("refused streaming run wrote data")
	}
}

func TestStreamingReservedTableRefusedInPlan(t *testing.T) {
	dir := t.TempDir()
	cfg, key := streamingFixtureFiles(t, []string{"a@example.invalid"},
		filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db"))
	cfg.Table = "maskriver_outbox_batch"
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err := engine.Plan(context.Background()); err == nil {
		t.Fatal("reserved outbox table planned as a business scope")
	}
	if _, err := engine.Scan(context.Background(), contracts.ScanRequest{}); err == nil {
		t.Fatal("reserved outbox table scanned as a business scope")
	}
}
