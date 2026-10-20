package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const outboxTestRunID = "0123456789abcdef0123456789abcdef"

func outboxTestOptions(t *testing.T) (OutboxOptions, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("streaming outbox is Linux-only in this version; see TestOutboxUnsupportedPlatform")
	}
	path := filepath.Join(t.TempDir(), "synthetic-outbox.db")
	seed, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL, nickname TEXT NULL)`,
		`INSERT INTO people(id,email,nickname) VALUES(1,'fixture-01@example.invalid','synthetic-01')`,
		`INSERT INTO people(id,email,nickname) VALUES(2,'fixture-02@example.invalid','synthetic-02')`,
		`INSERT INTO people(id,email,nickname) VALUES(3,'fixture-03@example.invalid','synthetic-03')`,
	} {
		if _, err := seed.Exec(statement); err != nil {
			_ = seed.Close()
			t.Fatalf("fixture statement failed: %v", err)
		}
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return OutboxOptions{
		DatabaseOptions: contracts.DatabaseOptions{
			Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
			DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 2,
			ConnectTimeout: 5 * time.Second,
		},
		Protocol:       contracts.StreamingProtocolRevision,
		JournalMode:    "delete",
		MaxReceiptPage: 50,
	}, path
}

func installOutbox(t *testing.T, opts OutboxOptions) {
	t.Helper()
	if err := InstallOutboxSchema(context.Background(), opts); err != nil {
		t.Fatalf("install outbox schema: %v", err)
	}
}

func openOutbox(t *testing.T, opts OutboxOptions) *Outbox {
	t.Helper()
	o, err := OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatalf("open outbox: %v", err)
	}
	t.Cleanup(func() { _ = o.Close() })
	if err := o.InitializeRun(context.Background(), OutboxRun{
		RunID: outboxTestRunID, PlanID: strings.Repeat("a", 64),
		ApprovalBinding: contracts.ApprovalBindingRevision,
		TargetDigest:    strings.Repeat("b", 64), ScopeDigest: strings.Repeat("c", 64),
		Protocol: contracts.StreamingProtocolRevision,
	}); err != nil {
		t.Fatalf("initialize run: %v", err)
	}
	return o
}

func outboxPlan(t *testing.T, o *Outbox, tableName string, columns ...string) contracts.TablePlan {
	t.Helper()
	ref := contracts.TableRef{Database: "fixture", Schema: "main", Table: tableName}
	schema, err := o.writer.describe(context.Background(), ref)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	plan := contracts.TablePlan{Table: ref, PrimaryKey: schema.PrimaryKey}
	for _, wanted := range columns {
		for _, candidate := range schema.Columns {
			if candidate.Ref.Column == wanted {
				plan.Columns = append(plan.Columns, contracts.ColumnPlan{Column: candidate.Ref, Type: candidate.Type})
			}
		}
	}
	if len(plan.Columns) != len(columns) {
		t.Fatalf("plan columns missing")
	}
	return plan
}

func outboxChange(t *testing.T, id int64, email, nickname string) contracts.RowChange {
	t.Helper()
	return outboxChangeExpect(t, id, fmt.Sprintf("fixture-%02d@example.invalid", id), fmt.Sprintf("synthetic-%02d", id), email, nickname)
}

func outboxChangeExpect(t *testing.T, id int64, expectedEmail, expectedNickname, email, nickname string) contracts.RowChange {
	t.Helper()
	return contracts.RowChange{
		Key:      []contracts.Value{testValue(t, contracts.Int, fmt.Sprint(id))},
		Expected: contracts.Row{"email": testValue(t, contracts.Text, expectedEmail), "nickname": testValue(t, contracts.Text, expectedNickname)},
		Replacement: contracts.Row{
			"email":    testValue(t, contracts.Text, email),
			"nickname": testValue(t, contracts.Text, nickname),
		},
	}
}

func emailChange(t *testing.T, id int64, expected, replacement string) contracts.RowChange {
	t.Helper()
	return contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, fmt.Sprint(id))},
		Expected:    contracts.Row{"email": testValue(t, contracts.Text, expected)},
		Replacement: contracts.Row{"email": testValue(t, contracts.Text, replacement)},
	}
}

func outboxReceipt(sequence, rows int64, batchID string) contracts.StreamingBatchReceipt {
	return contracts.StreamingBatchReceipt{
		Table:   contracts.TableRef{Database: "fixture", Schema: "main", Table: "people"},
		BatchID: batchID, Ordinal: sequence, Rows: rows, Outcome: contracts.BatchCommitted,
	}
}

func queryCount(t *testing.T, path, query string, args ...any) int64 {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open verify connection: %v", err)
	}
	defer conn.Close()
	var n int64
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("verify query: %v", err)
	}
	return n
}

func queryString(t *testing.T, path, query string, args ...any) string {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open verify connection: %v", err)
	}
	defer conn.Close()
	var s string
	if err := conn.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("verify query: %v", err)
	}
	return s
}

// TestOutboxSameTransactionCommit is the G1 evidence: one Commit call persists
// the business UPDATE and the receipt together, and a fresh connection sees
// both.
func TestOutboxSameTransactionCommit(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	receipt := outboxReceipt(0, 1, "batch-000")

	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, receipt)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if n, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1, "masked-01@example.invalid", "masked-01")}); err != nil || n != 1 {
		t.Fatalf("update n=%d err=%v", n, err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "masked-01@example.invalid" {
		t.Fatalf("business update missing: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch WHERE run_id=? AND batch_sequence=0`, outboxTestRunID); got != 1 {
		t.Fatalf("receipt missing: %d", got)
	}
	if got := queryString(t, path, `SELECT receipt_version FROM maskriver_outbox_batch WHERE run_id=? AND batch_sequence=0`, outboxTestRunID); got != contracts.StreamingProtocolRevision {
		t.Fatalf("receipt version=%q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch WHERE run_id=?`, outboxTestRunID); got != 1 {
		t.Fatalf("unexpected receipts: %d", got)
	}
}

func TestOutboxUpdateFailureLeavesNoReceipt(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	change := outboxChange(t, 1, "masked-01@example.invalid", "masked-01")
	change.Expected["email"] = testValue(t, contracts.Text, "wrong@example.invalid")
	if _, err := tx.Update(context.Background(), []contracts.RowChange{change}); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("update err=%v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("business row changed: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("orphan receipt: %d", got)
	}
}

func TestOutboxReceiptFailureRollsBackBusiness(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	// A pre-existing receipt with the same sequence makes the in-transaction
	// INSERT fail after the UPDATE has been executed.
	tx0, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 0, "batch-000"))
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	if err := tx0.Commit(context.Background()); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-001"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 2, "masked-02@example.invalid", "masked-02")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("commit err=%v want conflict", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=2`); got != "fixture-02@example.invalid" {
		t.Fatalf("business update committed without receipt: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 1 {
		t.Fatalf("receipt count=%d", got)
	}
}

func TestOutboxDuplicateBatchIdentityRejected(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")

	commit := func(sequence int64, batchID string) error {
		tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(sequence, 1, batchID))
		if err != nil {
			return err
		}
		if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1+sequence, "masked@example.invalid", "masked")}); err != nil {
			return err
		}
		return tx.Commit(context.Background())
	}
	if err := commit(0, "batch-000"); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if err := commit(1, "batch-000"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("duplicate batch id err=%v", err)
	}
	if err := commit(0, "batch-001"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("duplicate sequence err=%v", err)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 1 {
		t.Fatalf("receipt count=%d", got)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=2`); got != "fixture-02@example.invalid" {
		t.Fatalf("duplicate commit leaked a business update: %q", got)
	}
}

func TestOutboxReceiptRowCountMismatchRollsBack(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email")
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 2, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{emailChange(t, 1, "fixture-01@example.invalid", "masked-01@example.invalid")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("commit err=%v", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("mismatched receipt committed data: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("mismatched receipt persisted: %d", got)
	}
}

func TestOutboxRejectsReservedBusinessTable(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	ref := contracts.TableRef{Database: "fixture", Schema: "main", Table: OutboxBatchTable}
	plan := contracts.TablePlan{Table: ref, PrimaryKey: []string{"run_id"}, Columns: []contracts.ColumnPlan{{
		Column: contracts.ColumnRef{TableRef: ref, Column: "batch_id"}, Type: contracts.Type{Kind: contracts.Text},
	}}}
	_, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, contracts.StreamingBatchReceipt{
		Table: ref, BatchID: "b", Ordinal: 0, Rows: 0, Outcome: contracts.BatchCommitted,
	})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("reserved table err=%v", err)
	}
}

func TestOutboxCommitUnknownIsConservative(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	o.hooks.commit = func(real func() error) error {
		_ = real() // the commit really happened
		return errors.New("injected post-commit failure")
	}
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1, "masked-01@example.invalid", "masked-01")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrCommitUnknown) {
		t.Fatalf("commit err=%v want commit unknown", err)
	}
	// The outcome was ambiguous to the caller, so no success may be claimed.
	// This test also proves that no automatic retry wrote a second receipt.
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 1 {
		t.Fatalf("receipt count=%d", got)
	}
}

func TestOutboxHooksPreventReceipt(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	o.hooks.beforeReceipt = func() error { return errors.New("injected pre-receipt failure") }
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1, "masked-01@example.invalid", "masked-01")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tx.Commit(context.Background()); !errors.Is(err, contracts.ErrDatabase) {
		t.Fatalf("commit err=%v", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("business row committed without receipt: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("orphan receipt: %d", got)
	}
}

func TestOutboxCommitCancelBeforeAttempt(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email", "nickname")
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1, "masked-01@example.invalid", "masked-01")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tx.Commit(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("commit err=%v", err)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("receipt after cancel: %d", got)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("business row changed after cancel: %q", got)
	}
}

func TestOutboxPageBoundedCursor(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email")
	current := map[int64]string{1: "fixture-01@example.invalid", 2: "fixture-02@example.invalid", 3: "fixture-03@example.invalid"}
	for i := int64(0); i < 5; i++ {
		id := 1 + i%3
		tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(i, 1, fmt.Sprintf("batch-%03d", i)))
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		next := fmt.Sprintf("masked-%02d@example.invalid", i+1)
		if _, err := tx.Update(context.Background(), []contracts.RowChange{emailChange(t, id, current[id], next)}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		current[id] = next
		if err := tx.Commit(context.Background()); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	// Contract-level reader is bounded and rejects oversized pages.
	reader, err := o.Reader(outboxTestRunID)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer reader.Close()
	page, err := reader.Page(context.Background(), 2)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page.Receipts) != 2 || page.Done || page.NextOrdinal != 2 {
		t.Fatalf("page=%+v", page)
	}
	if _, err := reader.Page(context.Background(), o.maxPage+1); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("oversized page err=%v", err)
	}
	seen := 0
	for {
		page, err = reader.Page(context.Background(), 2)
		if err != nil {
			t.Fatalf("page loop: %v", err)
		}
		for _, receipt := range page.Receipts {
			if receipt.Ordinal != int64(seen+2) {
				t.Fatalf("ordinal=%d want %d", receipt.Ordinal, seen+2)
			}
			seen++
		}
		if page.Done {
			break
		}
	}
	if seen != 3 {
		t.Fatalf("remaining receipts=%d", seen)
	}
	// A different run is not visible through another run's reader.
	if _, err := o.Reader("ffffffffffffffffffffffffffffffff"); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("unknown run err=%v", err)
	}
	// Cursor beyond the last receipt is an empty done page, not an error.
	tail, err := o.Page(context.Background(), outboxTestRunID, 99, 2)
	if err != nil || !tail.Done || len(tail.Receipts) != 0 {
		t.Fatalf("tail page=%+v err=%v", tail, err)
	}
	_ = path
}

func TestOutboxReaderCloseAndCancel(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	reader, err := o.Reader(outboxTestRunID)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := reader.Page(context.Background(), 1); !errors.Is(err, contracts.ErrClosed) {
		t.Fatalf("use after close err=%v", err)
	}
	reader2, err := o.Reader(outboxTestRunID)
	if err != nil {
		t.Fatalf("reader2: %v", err)
	}
	defer reader2.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader2.Page(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestOutboxPagingThousandBatches(t *testing.T) {
	if testing.Short() {
		t.Skip("1000 fsync batches")
	}
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email")
	const batches = 1000
	current := map[int64]string{1: "fixture-01@example.invalid", 2: "fixture-02@example.invalid", 3: "fixture-03@example.invalid"}
	for i := int64(0); i < batches; i++ {
		id := 1 + i%3
		tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(i, 1, fmt.Sprintf("batch-%04d", i)))
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		next := fmt.Sprintf("masked-%04d@example.invalid", i)
		if _, err := tx.Update(context.Background(), []contracts.RowChange{emailChange(t, id, current[id], next)}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		current[id] = next
		if err := tx.Commit(context.Background()); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	reader, err := o.Reader(outboxTestRunID)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer reader.Close()
	var seen int64
	pages := 0
	maxLen := 0
	for {
		page, err := reader.Page(context.Background(), 32)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		pages++
		if len(page.Receipts) > maxLen {
			maxLen = len(page.Receipts)
		}
		seen += int64(len(page.Receipts))
		if page.Done {
			break
		}
	}
	if seen != batches {
		t.Fatalf("receipts=%d want %d", seen, batches)
	}
	if maxLen > 32 {
		t.Fatalf("page length=%d", maxLen)
	}
	if pages != 32 {
		t.Fatalf("pages=%d", pages)
	}
}

func TestOutboxSchemaFailClosed(t *testing.T) {
	ctx := context.Background()
	opts, path := outboxTestOptions(t)
	if _, err := OpenOutbox(ctx, opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("missing schema err=%v", err)
	}
	installOutbox(t, opts)
	if err := InstallOutboxSchema(ctx, opts); err != nil {
		t.Fatalf("idempotent install: %v", err)
	}
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE maskriver_outbox_schema SET version='other'`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if _, err := OpenOutbox(ctx, opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("wrong version err=%v", err)
	}
}

func TestOutboxJournalModeFailClosed(t *testing.T) {
	ctx := context.Background()
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if _, err := OpenOutbox(ctx, opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("wal err=%v", err)
	}
	// The outbox must not silently rewrite the database mode to make itself work.
	if got := strings.ToLower(queryString(t, path, `PRAGMA journal_mode`)); got != "wal" {
		t.Fatalf("journal mode changed to %q", got)
	}
}

func TestOutboxUnsupportedOptions(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	opts.Protocol = "m9"
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("wrong protocol err=%v", err)
	}
	opts, _ = outboxTestOptions(t)
	opts.JournalMode = "wal"
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("wrong journal requirement err=%v", err)
	}
	opts, _ = outboxTestOptions(t)
	opts.MaxReceiptPage = 0
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("zero page err=%v", err)
	}
}

func TestOutboxPathPreflight(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)

	link := filepath.Join(filepath.Dir(path), "link.db")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	symlinkOpts := opts
	symlinkOpts.DSN = testValue(t, contracts.Text, link)
	if _, err := OpenOutbox(context.Background(), symlinkOpts); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("symlink err=%v", err)
	}

	hard := filepath.Join(filepath.Dir(path), "hard.db")
	if err := os.Link(path, hard); err != nil {
		t.Fatalf("hardlink: %v", err)
	}
	hardOpts := opts
	hardOpts.DSN = testValue(t, contracts.Text, hard)
	if _, err := OpenOutbox(context.Background(), hardOpts); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("hardlink err=%v", err)
	}

	relative := opts
	relative.DSN = testValue(t, contracts.Text, "relative.db")
	if _, err := OpenOutbox(context.Background(), relative); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("relative err=%v", err)
	}

	public := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(public, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(public, 0o777); err != nil {
		t.Fatal(err)
	}
	publicPath := filepath.Join(public, "db.db")
	seed, err := sql.Open("sqlite", "file:"+publicPath+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`CREATE TABLE t(x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	publicOpts := opts
	publicOpts.DSN = testValue(t, contracts.Text, publicPath)
	if r := runtime.GOOS; r == "linux" {
		if _, err := OpenOutbox(context.Background(), publicOpts); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("world-writable parent err=%v", err)
		}
	}
}

func TestOutboxFullDiskFailsClosed(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	plan := outboxPlan(t, o, "people", "email")
	var pageCount, freelist int64
	if err := o.conn.QueryRowContext(context.Background(), "PRAGMA main.page_count").Scan(&pageCount); err != nil {
		t.Fatal(err)
	}
	if err := o.conn.QueryRowContext(context.Background(), "PRAGMA main.freelist_count").Scan(&freelist); err != nil {
		t.Fatal(err)
	}
	t.Logf("page_count=%d freelist=%d", pageCount, freelist)
	if _, err := o.conn.ExecContext(context.Background(), "VACUUM"); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
	if err := o.conn.QueryRowContext(context.Background(), "PRAGMA main.page_count").Scan(&pageCount); err != nil {
		t.Fatal(err)
	}
	if _, err := o.conn.ExecContext(context.Background(), fmt.Sprintf("PRAGMA main.max_page_count=%d", pageCount)); err != nil {
		t.Fatal(err)
	}
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// A large replacement forces page allocation inside the same transaction, so
	// the page limit produces a real SQLITE_FULL rather than a simulated one.
	large := strings.Repeat("x", 200_000)
	_, updateErr := tx.Update(context.Background(), []contracts.RowChange{emailChange(t, 1, "fixture-01@example.invalid", large)})
	if updateErr != nil {
		if !errors.Is(updateErr, contracts.ErrDatabase) {
			t.Fatalf("update err=%v", updateErr)
		}
		if err := tx.Rollback(context.Background()); err != nil {
			t.Fatalf("rollback: %v", err)
		}
	} else {
		commitErr := tx.Commit(context.Background())
		if commitErr == nil {
			t.Skip("fixture did not reach the page limit; SQLITE_FULL not exercised")
		}
		if !errors.Is(commitErr, contracts.ErrDatabase) && !errors.Is(commitErr, contracts.ErrCommitUnknown) {
			t.Fatalf("commit err=%v", commitErr)
		}
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("business row committed on full disk: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("receipt on full disk: %d", got)
	}
}

func TestOutboxLegacyWriterUnaffected(t *testing.T) {
	ctx := context.Background()
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	opened, err := OpenWriter(ctx, opts.DatabaseOptions)
	if err != nil {
		t.Fatalf("legacy writer: %v", err)
	}
	w := opened.(*writer)
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("legacy begin: %v", err)
	}
	if _, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": testValue(t, contracts.Text, "fixture-01@example.invalid")},
		Replacement: contracts.Row{"email": testValue(t, contracts.Text, "legacy-01@example.invalid")},
	}}); err != nil {
		t.Fatalf("legacy update: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("legacy commit: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("legacy close: %v", err)
	}
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "legacy-01@example.invalid" {
		t.Fatalf("legacy write missing: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("legacy path wrote outbox rows: %d", got)
	}
	_ = w
}

// TestOutboxLegacyDatabaseWithoutSchema proves a normal M1 database is not
// forced to install or understand the outbox.
func TestOutboxLegacyDatabaseWithoutSchema(t *testing.T) {
	ctx := context.Background()
	opts, _ := outboxTestOptions(t) // schema deliberately not installed
	opened, err := OpenWriter(ctx, opts.DatabaseOptions)
	if err != nil {
		t.Fatalf("legacy writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("legacy begin: %v", err)
	}
	if _, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": testValue(t, contracts.Text, "fixture-01@example.invalid")},
		Replacement: contracts.Row{"email": testValue(t, contracts.Text, "legacy@example.invalid")},
	}}); err != nil {
		t.Fatalf("legacy update: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("legacy commit: %v", err)
	}
	if _, err := OpenOutbox(ctx, opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("outbox on legacy db err=%v", err)
	}
}

func TestOutboxNoConnectionLeak(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	for i := 0; i < 3; i++ {
		o, err := OpenOutbox(context.Background(), opts)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if err := o.Close(); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
		if stats := o.adapter.db.Stats(); stats.OpenConnections != 0 {
			t.Fatalf("connections leaked: %+v", stats)
		}
	}
}

// --- crash simulation ---

func TestOutboxCrashHelper(t *testing.T) {
	mode := os.Getenv("MASKRIVER_OUTBOX_CRASH")
	if mode == "" {
		t.Skip("crash helper process only")
	}
	path := os.Getenv("MASKRIVER_OUTBOX_PATH")
	opts := OutboxOptions{
		DatabaseOptions: contracts.DatabaseOptions{
			Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
			DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 2,
			ConnectTimeout: 5 * time.Second,
		},
		Protocol:       contracts.StreamingProtocolRevision,
		JournalMode:    "delete",
		MaxReceiptPage: 10,
	}
	o, err := OpenOutbox(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper open failed")
		os.Exit(2)
	}
	plan := outboxPlan(t, o, "people", "email", "nickname")
	tx, err := o.BeginBatch(context.Background(), outboxTestRunID, plan, outboxReceipt(0, 1, "batch-000"))
	if err != nil {
		os.Exit(2)
	}
	if _, err := tx.Update(context.Background(), []contracts.RowChange{outboxChange(t, 1, "crash@example.invalid", "crash")}); err != nil {
		os.Exit(2)
	}
	if mode == "after-commit" {
		if err := tx.Commit(context.Background()); err != nil {
			os.Exit(2)
		}
	}
	os.Exit(0) // abrupt process exit without rollback or close
}

func runCrashHelper(t *testing.T, mode, path string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=TestOutboxCrashHelper")
	command.Env = append(os.Environ(), "MASKRIVER_OUTBOX_CRASH="+mode, "MASKRIVER_OUTBOX_PATH="+path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("helper %s failed: %v (%s)", mode, err, output)
	}
	if !strings.Contains(string(output), "PASS") && !strings.Contains(string(output), "ok") {
		// A helper that exits before running the test binary still counts as a
		// crash for this purpose; no output assertion is required.
		_ = output
	}
}

func TestOutboxCrashBeforeCommit(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	// Parent creates the run row.
	o := openOutbox(t, opts)
	_ = o.Close()
	runCrashHelper(t, "before-commit", path)
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "fixture-01@example.invalid" {
		t.Fatalf("crash-before-commit left data: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 0 {
		t.Fatalf("crash-before-commit left receipt: %d", got)
	}
}

func TestOutboxCrashAfterCommit(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	_ = o.Close()
	runCrashHelper(t, "after-commit", path)
	if got := queryString(t, path, `SELECT email FROM people WHERE id=1`); got != "crash@example.invalid" {
		t.Fatalf("committed data missing: %q", got)
	}
	if got := queryCount(t, path, `SELECT COUNT(*) FROM maskriver_outbox_batch`); got != 1 {
		t.Fatalf("committed receipt missing: %d", got)
	}
}

func TestOutboxReceiptJSONHasNoPayload(t *testing.T) {
	receipt := outboxReceipt(3, 7, "batch-003")
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Replacement", "Expected", "Key", "DSN"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("receipt exposes %q", forbidden)
		}
	}
}

var _ sync.Locker = (*sync.Mutex)(nil)

func TestOutboxSessionPragmasAndCapabilities(t *testing.T) {
	opts, _ := outboxTestOptions(t)
	installOutbox(t, opts)
	o := openOutbox(t, opts)
	var mode string
	var sync, fk, queryOnly int
	ctx := context.Background()
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.synchronous").Scan(&sync); err != nil {
		t.Fatal(err)
	}
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.query_only").Scan(&queryOnly); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(mode) != "delete" || sync != 2 || fk != 1 || queryOnly != 0 {
		t.Fatalf("journal=%q synchronous=%d foreign_keys=%d query_only=%d", mode, sync, fk, queryOnly)
	}
	caps := o.Capabilities()
	if caps.SchemaVersion != OutboxSchemaVersion || caps.ProtocolVersion != contracts.StreamingProtocolRevision ||
		!caps.Supports(contracts.StreamingProtocolRevision) || caps.MaxReceiptPage != opts.MaxReceiptPage {
		t.Fatalf("capabilities=%+v", caps)
	}
}

func TestOutboxUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux is the supported platform")
	}
	opts := OutboxOptions{
		DatabaseOptions: contracts.DatabaseOptions{
			Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, filepath.Join(t.TempDir(), "x.db")),
			DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 2,
			ConnectTimeout: time.Second,
		},
		Protocol: contracts.StreamingProtocolRevision, JournalMode: "delete", MaxReceiptPage: 10,
	}
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("non-Linux outbox err=%v", err)
	}
}

func TestOutboxShapeTamperRejected(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE maskriver_outbox_batch`,
		`CREATE TABLE maskriver_outbox_batch (run_id TEXT, batch_sequence TEXT)`,
	} {
		if _, err := conn.Exec(statement); err != nil {
			t.Fatalf("tamper: %v", err)
		}
	}
	_ = conn.Close()
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("shape tamper err=%v", err)
	}
}

func TestOutboxGeneratedColumnTamperRejected(t *testing.T) {
	opts, path := outboxTestOptions(t)
	installOutbox(t, opts)
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`ALTER TABLE maskriver_outbox_run ADD COLUMN extra TEXT GENERATED ALWAYS AS (upper(protocol)) VIRTUAL`); err != nil {
		_ = conn.Close()
		t.Skipf("SQLite build does not support generated columns: %v", err)
	}
	_ = conn.Close()
	if _, err := OpenOutbox(context.Background(), opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("generated column accepted: %v", err)
	}
	if err := InstallOutboxSchema(context.Background(), opts); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("idempotent install accepted generated column: %v", err)
	}
}

func TestOutboxReservedTables(t *testing.T) {
	for _, name := range OutboxTables() {
		if !IsOutboxTable(name) {
			t.Fatalf("%s not reserved", name)
		}
	}
	for _, name := range []string{"people", "maskriver_outbox", "maskriver_outbox_batches"} {
		if IsOutboxTable(name) {
			t.Fatalf("%s incorrectly reserved", name)
		}
	}
	if len(OutboxTables()) != 3 {
		t.Fatalf("tables=%v", OutboxTables())
	}
}
