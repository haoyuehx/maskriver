package db

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func recoveryTestOptions(t *testing.T) (OutboxOptions, string) {
	t.Helper()
	opts, path := outboxTestOptions(t)
	if err := InstallOutboxSchema(context.Background(), opts); err != nil {
		t.Fatalf("install outbox: %v", err)
	}
	if err := InstallRecoverySchema(context.Background(), opts); err != nil {
		t.Fatalf("install recovery: %v", err)
	}
	return opts, path
}

func recoveryTestManifest(runID string) RecoveryManifest {
	return RecoveryManifest{
		RunID: runID, Protocol: RecoveryProtocolRevision,
		StreamingProtocol: contracts.StreamingProtocolRevision,
		ApprovalBinding:   contracts.ApprovalBindingRevision,
		PlanID:            strings.Repeat("a", 64), PlanContentDigest: strings.Repeat("b", 64),
		ScopeDigest: strings.Repeat("c", 64), SourceSnapshotDigest: strings.Repeat("d", 64),
		TargetID: strings.Repeat("e", 64), TargetIdentity: "1:2",
		SchemaDigest: strings.Repeat("f", 64), ConfigDigest: strings.Repeat("1", 64),
		StrategyID: "redact", StrategyVersion: "1", NormalizationVersion: "typed-lexical-v1",
		KeyID: "fixture-key", KeyCheck: strings.Repeat("3", 64), SettingsDigest: strings.Repeat("2", 64),
		State: RecoveryStateNew, Validation: ValidationNone,
	}
}

func openRecoveryFixture(t *testing.T, runID string) (*Recovery, *Outbox, string) {
	t.Helper()
	opts, path := recoveryTestOptions(t)
	o, err := OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatalf("open outbox: %v", err)
	}
	t.Cleanup(func() { _ = o.Close() })
	if err := o.InitializeRun(context.Background(), OutboxRun{
		RunID: runID, PlanID: strings.Repeat("a", 64),
		ApprovalBinding: contracts.ApprovalBindingRevision,
		TargetDigest:    strings.Repeat("e", 64), ScopeDigest: strings.Repeat("c", 64),
		Protocol: contracts.StreamingProtocolRevision,
	}); err != nil {
		t.Fatalf("initialize run: %v", err)
	}
	recovery, err := o.OpenRecovery(context.Background())
	if err != nil {
		t.Fatalf("open recovery: %v", err)
	}
	if err := recovery.CreateManifest(context.Background(), recoveryTestManifest(runID)); err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	return recovery, o, path
}

func recoveryTestProgress(runID string, seq int64, kind string, ordinal int, processed, committed, batches, affected int64, batchID string, business bool) RecoveryProgress {
	return RecoveryProgress{
		RunID: runID, Sequence: seq, Kind: kind, TableOrdinal: ordinal,
		RowsProcessed: processed, RowsCommitted: committed, CommittedBatches: batches,
		BatchID: batchID, AffectedRows: affected, BusinessComplete: business,
	}
}

func TestRecoverySchemaInstallIdempotentAndVersioned(t *testing.T) {
	opts, _ := recoveryTestOptions(t)
	if err := InstallRecoverySchema(context.Background(), opts); err != nil {
		t.Fatalf("second install must be a no-op: %v", err)
	}
	// Recovery can not be installed on a database without the outbox schema.
	bare, _ := outboxTestOptions(t)
	if err := InstallRecoverySchema(context.Background(), bare); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("recovery without outbox: got %v, want ErrUnsupported", err)
	}
	// A different version marker is refused, never repaired.
	a, err := open(context.Background(), opts.DatabaseOptions, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.db.ExecContext(context.Background(), "UPDATE main."+recoverySchemaTable+" SET version='maskriver-recovery-v0'"); err != nil {
		t.Fatal(err)
	}
	if err := InstallRecoverySchema(context.Background(), opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("wrong recovery version: got %v, want ErrUnsupported", err)
	}
}

func TestRecoveryReservedTables(t *testing.T) {
	for _, name := range RecoveryTables() {
		if !IsOutboxTable(name) {
			t.Fatalf("%s must be reserved", name)
		}
	}
}

func TestRecoveryBatchSameTransaction(t *testing.T) {
	runID := outboxTestRunID
	recovery, o, path := openRecoveryFixture(t, runID)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	receipt := outboxReceipt(0, 2, runID+"-0")
	progress := recoveryTestProgress(runID, 1, "batch", 0, 2, 2, 1, 2, receipt.BatchID, false)
	tx, err := recovery.BeginRecoveryBatch(context.Background(), runID, plan, receipt, progress)
	if err != nil {
		t.Fatalf("begin recovery batch: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		outboxChange(t, 1, "masked-01@example.invalid", "nick-01"),
		outboxChange(t, 2, "masked-02@example.invalid", "nick-02"),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := queryCount(t, path, "SELECT COUNT(*) FROM maskriver_outbox_batch"); got != 1 {
		t.Fatalf("receipts=%d, want 1", got)
	}
	if got := queryCount(t, path, "SELECT COUNT(*) FROM maskriver_recovery_progress"); got != 1 {
		t.Fatalf("progress rows=%d, want 1", got)
	}
	m, err := recovery.Manifest(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if m.CheckpointSequence != 1 || m.State != RecoveryStateRunning {
		t.Fatalf("manifest sequence=%d state=%s", m.CheckpointSequence, m.State)
	}
	// The same sequence can never be committed again.
	again := recoveryTestProgress(runID, 1, "progress", 0, 2, 2, 1, 0, "", false)
	if err := recovery.CommitProgress(context.Background(), runID, again); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("duplicate sequence: got %v, want ErrConflict", err)
	}
	// A gap is refused.
	gap := recoveryTestProgress(runID, 3, "progress", 0, 3, 2, 1, 0, "", false)
	if err := recovery.CommitProgress(context.Background(), runID, gap); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("sequence gap: got %v, want ErrConflict", err)
	}
}

func TestRecoveryCheckpointFailureRollsBackBusiness(t *testing.T) {
	runID := outboxTestRunID
	recovery, o, path := openRecoveryFixture(t, runID)
	plan := outboxPlan(t, o, "people", "email")
	receipt := outboxReceipt(0, 1, runID+"-0")
	// RowsCommitted understates the affected rows, so the in-transaction
	// checkpoint update fails after the business UPDATE and the receipt INSERT.
	bad := recoveryTestProgress(runID, 1, "batch", 0, 1, 0, 1, 1, receipt.BatchID, false)
	tx, err := recovery.BeginRecoveryBatch(context.Background(), runID, plan, receipt, bad)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		emailChange(t, 1, "fixture-01@example.invalid", "masked-01@example.invalid"),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("commit: got %v, want ErrConflict", err)
	}
	// All three effects must be absent.
	if got := queryCount(t, path, "SELECT COUNT(*) FROM maskriver_outbox_batch"); got != 0 {
		t.Fatalf("receipt persisted after rollback: %d", got)
	}
	if got := queryCount(t, path, "SELECT COUNT(*) FROM maskriver_recovery_progress"); got != 0 {
		t.Fatalf("progress persisted after rollback: %d", got)
	}
	if got := queryCount(t, path, "SELECT COUNT(*) FROM people WHERE email='fixture-01@example.invalid'"); got != 1 {
		t.Fatalf("business update persisted after rollback")
	}
}

func TestRecoveryCommitUnknownAfterRealCommit(t *testing.T) {
	runID := outboxTestRunID
	recovery, o, _ := openRecoveryFixture(t, runID)
	plan := outboxPlan(t, o, "people", "email")
	receipt := outboxReceipt(0, 1, runID+"-0")
	progress := recoveryTestProgress(runID, 1, "batch", 0, 1, 1, 1, 1, receipt.BatchID, false)
	// The hook performs the real commit and then reports an error, modelling
	// "the database committed but the application lost the response".
	o.hooks.commit = func(real func() error) error {
		if err := real(); err != nil {
			return err
		}
		return errors.New("simulated lost response")
	}
	tx, err := recovery.BeginRecoveryBatch(context.Background(), runID, plan, receipt, progress)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		emailChange(t, 1, "fixture-01@example.invalid", "masked-01@example.invalid"),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrCommitUnknown) {
		t.Fatalf("commit: got %v, want ErrCommitUnknown", err)
	}
	// The authoritative database advanced even though the caller saw an error.
	m, err := recovery.Manifest(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if m.CheckpointSequence != 1 {
		t.Fatalf("checkpoint=%d, want 1", m.CheckpointSequence)
	}
	next, _, err := recovery.ProgressPage(context.Background(), runID, 0, 10)
	if err != nil || len(next) != 1 || next[0].Sequence != 1 || next[0].BusinessComplete {
		t.Fatalf("progress mismatch: %v %+v", err, next)
	}
}

func TestRecoveryNoOpProgressAndBoundary(t *testing.T) {
	runID := outboxTestRunID
	recovery, _, path := openRecoveryFixture(t, runID)
	noop := recoveryTestProgress(runID, 1, "progress", 0, 3, 0, 0, 0, "", false)
	if err := recovery.CommitProgress(context.Background(), runID, noop); err != nil {
		t.Fatalf("no-op progress: %v", err)
	}
	if got := queryCount(t, path, "SELECT COUNT(*) FROM maskriver_outbox_batch"); got != 0 {
		t.Fatalf("no-op progress wrote receipts: %d", got)
	}
	boundary := recoveryTestProgress(runID, 2, "boundary", 1, 0, 0, 0, 0, "", true)
	if err := recovery.CommitProgress(context.Background(), runID, boundary); err != nil {
		t.Fatalf("boundary: %v", err)
	}
	m, err := recovery.Manifest(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if !m.BusinessComplete || m.CheckpointSequence != 2 {
		t.Fatalf("manifest business_complete=%t sequence=%d", m.BusinessComplete, m.CheckpointSequence)
	}
	// Once business is complete, a new business/progress write is refused.
	after := recoveryTestProgress(runID, 3, "progress", 1, 1, 0, 0, 0, "", false)
	if err := recovery.CommitProgress(context.Background(), runID, after); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("post-complete progress: got %v, want ErrConflict", err)
	}
}

func TestRecoveryTamperedProgressIsVisibleNotRepaired(t *testing.T) {
	runID := outboxTestRunID
	recovery, o, path := openRecoveryFixture(t, runID)
	plan := outboxPlan(t, o, "people", "email")
	receipt := outboxReceipt(0, 1, runID+"-0")
	progress := recoveryTestProgress(runID, 1, "batch", 0, 1, 1, 1, 1, receipt.BatchID, false)
	tx, err := recovery.BeginRecoveryBatch(context.Background(), runID, plan, receipt, progress)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		emailChange(t, 1, "fixture-01@example.invalid", "masked-01@example.invalid"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A hostile writer with database access can append a gap; the reader must
	// expose it rather than silently repairing or hiding it.
	if _, err := o.conn.ExecContext(context.Background(),
		"INSERT INTO main.maskriver_recovery_progress(run_id,checkpoint_sequence,kind,table_ordinal,rows_processed,rows_committed,committed_batches,batch_id,affected_rows,business_complete,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
		runID, 5, "progress", 0, 5, 1, 1, "", 0, 0, outboxTimestamp()); err != nil {
		t.Fatalf("inject gap: %v", err)
	}
	page, _, err := recovery.ProgressPage(context.Background(), runID, 1, 10)
	if err != nil || len(page) != 1 || page[0].Sequence != 5 {
		t.Fatalf("tampered progress not visible: %v %+v", err, page)
	}
	_ = path
}

func TestRecoveryLegacyOutboxIsNotRecoverable(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	available, err := o.RecoveryAvailable(context.Background())
	if err != nil || available {
		t.Fatalf("legacy outbox availability=%t err=%v", available, err)
	}
	if _, err := o.OpenRecovery(context.Background()); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("legacy OpenRecovery: got %v, want ErrUnsupported", err)
	}
	// The legacy v1 receipt path still works.
	plan := outboxPlan(t, o, "people", "email")
	receipt := outboxReceipt(0, 1, outboxTestRunID+"-0")
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		emailChange(t, 1, "fixture-01@example.invalid", "legacy-01@example.invalid"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverySchemaShapeTamperRejected(t *testing.T) {
	opts, path := recoveryTestOptions(t)
	conn, err := open(context.Background(), opts.DatabaseOptions, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.db.ExecContext(context.Background(), "DROP TABLE main."+RecoveryProgressTable); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.db.ExecContext(context.Background(),
		"CREATE TABLE main."+RecoveryProgressTable+" (run_id TEXT PRIMARY KEY, extra TEXT)"); err != nil {
		t.Fatal(err)
	}
	o, err := OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if _, err := o.OpenRecovery(context.Background()); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("tampered shape: got %v, want ErrUnsupported", err)
	}
	_ = path
}

func TestRecoveryReaderIsReadOnly(t *testing.T) {
	runID := outboxTestRunID
	recovery, o, path := openRecoveryFixture(t, runID)
	plan := outboxPlan(t, o, "people", "email")
	receipt := outboxReceipt(0, 1, runID+"-0")
	progress := recoveryTestProgress(runID, 1, "batch", 0, 1, 1, 1, 1, receipt.BatchID, false)
	tx, err := recovery.BeginRecoveryBatch(context.Background(), runID, plan, receipt, progress)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{
		emailChange(t, 1, "fixture-01@example.invalid", "masked-01@example.invalid"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, path)
	reader, err := OpenRecoveryReader(context.Background(), recoveryReaderOptions(t, o))
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	m, err := reader.Manifest(context.Background(), runID)
	if err != nil || m.CheckpointSequence != 1 {
		t.Fatalf("manifest: %v %+v", err, m)
	}
	p, ok, err := reader.LatestProgress(context.Background(), runID)
	if err != nil || !ok || p.Sequence != 1 {
		t.Fatalf("progress: %v ok=%t %+v", err, ok, p)
	}
	receipts, err := reader.Receipts(context.Background(), runID)
	if err != nil || receipts.Count != 1 || receipts.Rows != 1 {
		t.Fatalf("receipts: %v %+v", err, receipts)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	after := fileHash(t, path)
	if before != after {
		t.Fatalf("read-only status changed the database file")
	}
}

func recoveryReaderOptions(t *testing.T, o *Outbox) contracts.DatabaseOptions {
	t.Helper()
	return contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, o.path),
		DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 1,
		ConnectTimeout: 5 * time.Second,
	}
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func TestRecoveryManifestStateCASRejectsStaleSequence(t *testing.T) {
	runID := outboxTestRunID
	recovery, _, _ := openRecoveryFixture(t, runID)
	if err := recovery.SetManifestState(context.Background(), runID, 1,
		RecoveryStateRecoverable, false, ValidationNone); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("stale CAS: got %v, want ErrConflict", err)
	}
	if err := recovery.SetManifestState(context.Background(), runID, 0,
		RecoveryStateRunning, false, ValidationNone); err != nil {
		t.Fatalf("current CAS: %v", err)
	}
	// Terminal states are immutable within one run.
	if err := recovery.SetManifestState(context.Background(), runID, 0,
		RecoveryStateComplete, true, ValidationPassed); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := recovery.SetManifestState(context.Background(), runID, 0,
		RecoveryStateRunning, false, ValidationNone); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("terminal overwrite: got %v, want ErrConflict", err)
	}
}
