package runner

// Boundary tests for the streaming resource budget. They assert fail-closed
// behaviour and exact/over-limit edges, including a large row that appears only
// after a committed batch.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func customStreamingFixture(t *testing.T, statements []string, scopes []config.Scope) (*Engine, contracts.Plan, string) {
	t.Helper()
	requireLinuxStreaming(t)
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range statements {
			if _, err := database.Exec(statement); err != nil {
				_ = database.Close()
				t.Fatal(err)
			}
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	cfg := config.Runtime{
		Source: source, Target: target, Scopes: scopes,
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 64, MinRatio: .9,
		BatchSize: 64, Workers: 1,
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	plan, err := engine.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return engine, plan, target
}

func installOutboxFor(t *testing.T, engine *Engine) {
	t.Helper()
	opts, err := engine.streamingOutboxOptions(contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Budget: testBudget, ReceiptPageSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InstallOutboxSchema(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
}

var budgetRunCounter int

// freshRunID yields a distinct valid run identity per call; reusing one would
// make InitializeRun fail with a conflict instead of testing the budget.
func freshRunID() string {
	budgetRunCounter++
	return fmt.Sprintf("%032x", 0x1000+budgetRunCounter)
}

func runStreamingBudget(t *testing.T, engine *Engine, plan contracts.Plan, budget contracts.ResourceBudget) (contracts.StreamingApplySummary, error) {
	t.Helper()
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	return engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: freshRunID(),
		BatchSize: budget.MaxBatchSize, ReceiptPageSize: 8, Budget: budget,
	})
}

func TestStreamingRowBytesExactAndOver(t *testing.T) {
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
	}
	scopes := []config.Scope{{Table: "people", Column: "email"}}
	engine, plan, target := customStreamingFixture(t, statements, scopes)
	installOutboxFor(t, engine)
	// Projected row bytes = len("1") + len("a@example.invalid").
	exact := int64(len("1") + len("a@example.invalid"))

	over := testBudget
	over.MaxRowBytes = exact - 1
	if _, err := runStreamingBudget(t, engine, plan, over); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("over-limit row err=%v", err)
	}
	if got := readEmailColumn(t, target, "people"); got[1] != "a@example.invalid" {
		t.Fatalf("over-limit row wrote: %v", got)
	}

	ok := testBudget
	ok.MaxRowBytes = exact
	summary, err := runStreamingBudget(t, engine, plan, ok)
	if err != nil {
		t.Fatalf("exact-limit row: %v", err)
	}
	if summary.State != contracts.RunStateComplete {
		t.Fatalf("state=%v", summary.State)
	}
}

func TestStreamingRowBytesMultiColumnTotal(t *testing.T) {
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT, backup TEXT)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'b@example.invalid')`,
	}
	scopes := []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup"}}
	engine, plan, target := customStreamingFixture(t, statements, scopes)
	installOutboxFor(t, engine)
	each := int64(len("1") + len("a@example.invalid"))
	budget := testBudget
	// Each column fits, the projected row total does not.
	budget.MaxRowBytes = each + 1
	if _, err := runStreamingBudget(t, engine, plan, budget); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("multi-column total err=%v", err)
	}
	if got := readEmailColumn(t, target, "people"); got[1] != "a@example.invalid" {
		t.Fatalf("multi-column over-limit wrote: %v", got)
	}
}

func TestStreamingRowBytesLargeAfterCommittedBatch(t *testing.T) {
	huge := strings.Repeat("a", 50) + "@example.invalid"
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
		fmt.Sprintf(`INSERT INTO people VALUES (2, '%s')`, huge),
	}
	scopes := []config.Scope{{Table: "people", Column: "email"}}
	engine, plan, target := customStreamingFixture(t, statements, scopes)
	installOutboxFor(t, engine)
	engine.cfg.BatchSize = 1
	budget := testBudget
	budget.MaxBatchSize = 1
	budget.MaxRowBytes = 64
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: freshRunID(),
		BatchSize: 1, ReceiptPageSize: 8, Budget: budget,
	})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("late large row err=%v", err)
	}
	if summary.BatchesCommitted != 1 || summary.State == contracts.RunStateComplete {
		t.Fatalf("partial run summary=%+v", summary)
	}
	rows := readEmailColumn(t, target, "people")
	if rows[1] == "a@example.invalid" {
		t.Fatal("first batch was not committed before the failure")
	}
	if rows[2] != huge {
		t.Fatal("over-limit row was written")
	}
}

func TestStreamingRowBytesWorkersAndNullAndUTF8(t *testing.T) {
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, NULL)`,
		`INSERT INTO people VALUES (2, 'b@example.invalid')`,
	}
	scopes := []config.Scope{{Table: "people", Column: "email"}}
	engine, plan, _ := customStreamingFixture(t, statements, scopes)
	installOutboxFor(t, engine)
	engine.cfg.Workers = 4

	// NULL contributes nothing.
	rowBytes := int64(len("2") + len("b@example.invalid"))
	over := testBudget
	over.MaxWorkers = 4
	over.MaxRowBytes = rowBytes - 1
	if _, err := runStreamingBudget(t, engine, plan, over); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("worker budget err=%v", err)
	}
	ok := testBudget
	ok.MaxWorkers = 4
	ok.MaxRowBytes = rowBytes
	summary, err := runStreamingBudget(t, engine, plan, ok)
	if err != nil {
		t.Fatalf("exact budget: %v", err)
	}
	if summary.State != contracts.RunStateComplete {
		t.Fatalf("state=%v", summary.State)
	}

	// UTF-8 is measured in bytes, not runes.
	utf8Value, _ := contracts.NewValue(contracts.Text, "ü")
	utf8Row := contracts.Row{"id": mustText(t, "1"), "email": utf8Value}
	if _, err := pagePayloadBytes([]contracts.Row{utf8Row}, 3); err != nil {
		t.Fatalf("3-byte utf8 row rejected: %v", err)
	}
	if _, err := pagePayloadBytes([]contracts.Row{utf8Row}, 2); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("2-byte limit accepted a 3-byte row: %v", err)
	}
}

func mustText(t *testing.T, value string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Text, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRowPayloadBytesKinds(t *testing.T) {
	text, _ := contracts.NewValue(contracts.Text, "abc")
	blob, _ := contracts.NewValue(contracts.Bytes, "abcd")
	var null contracts.Value
	row := contracts.Row{"a": text, "b": blob, "c": null, "d": text}
	if got := rowPayloadBytes(row); got != 10 {
		t.Fatalf("rowPayloadBytes=%d want 10", got)
	}
}

func TestStreamingProcessAndDiskAdmission(t *testing.T) {
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
	}
	scopes := []config.Scope{{Table: "people", Column: "email"}}
	engine, plan, target := customStreamingFixture(t, statements, scopes)
	installOutboxFor(t, engine)

	t.Run("process combination", func(t *testing.T) {
		budget := testBudget
		budget.MaxProcessBytes = 1 << 20 // below baseline
		if _, err := runStreamingBudget(t, engine, plan, budget); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("process admission err=%v", err)
		}
		if got := readEmailColumn(t, target, "people"); got[1] != "a@example.invalid" {
			t.Fatalf("process refusal wrote: %v", got)
		}
	})
	t.Run("queue", func(t *testing.T) {
		budget := testBudget
		budget.MaxQueue = 1
		if _, err := runStreamingBudget(t, engine, plan, budget); !errors.Is(err, contracts.ErrUnsupported) {
			t.Fatalf("queue unsupported err=%v", err)
		}
	})
	t.Run("disk admission", func(t *testing.T) {
		budget := testBudget
		used, err := diskUsageBytes(target)
		if err != nil {
			t.Fatal(err)
		}
		budget.MaxDiskBytes = used + 1
		if _, err := runStreamingBudget(t, engine, plan, budget); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("disk admission err=%v", err)
		}
		if got := readEmailColumn(t, target, "people"); got[1] != "a@example.invalid" {
			t.Fatalf("disk refusal wrote: %v", got)
		}
	})
}

func TestCheckDiskAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target.db")
	if err := sqliteWriteFile(path, 4096); err != nil {
		t.Fatal(err)
	}
	if err := checkDiskAfterCommit(path, 2048); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("post-commit over-budget err=%v", err)
	}
	if err := checkDiskAfterCommit(path, 1<<20); err != nil {
		t.Fatalf("post-commit under-budget err=%v", err)
	}
}

func sqliteWriteFile(path string, size int) error {
	return os.WriteFile(path, make([]byte, size), 0o600)
}

// TestStreamingRowBytesPostMaterializationEvidence documents the remaining gap:
// the current Reader API materialises the row before any MaxRowBytes check can
// run. It is opt-in because it intentionally allocates a multi-megabyte value.
func TestStreamingRowBytesPostMaterializationEvidence(t *testing.T) {
	if os.Getenv("MASKRIVER_WP3F_LARGE_ROW") != "1" {
		t.Skip("set MASKRIVER_WP3F_LARGE_ROW=1 to measure the post-materialisation gap")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.db")
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", 8<<20) + "@example.invalid"
	if _, err := database.Exec(`INSERT INTO people VALUES (1, ?)`, huge); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	value, _ := contracts.NewValue(contracts.Text, path)
	reader, err := db.OpenReader(context.Background(), contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: value, DatasetID: "maskriver",
		Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ref := contracts.TableRef{Database: "maskriver", Schema: "main", Table: "people"}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	page, err := reader.Page(context.Background(), contracts.PageRequest{
		Table: ref, Columns: []string{"id", "email"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("rows=%d", len(page.Rows))
	}
	t.Logf("reader materialised a %d MiB field before any budget check: heap_delta=%d MiB",
		len(huge)>>20, (int64(after.HeapAlloc)-int64(before.HeapAlloc))>>20)
}
