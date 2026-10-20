package main

// End-to-end tests for the explicit m2 streaming path. Every database is a
// synthetic SQLite copy under t.TempDir(); each command runs in a real child
// process through the same main entry point.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const e2eRunID = "0123456789abcdef0123456789abcdef"

// requireLinuxOutbox skips streaming E2E on platforms where the SQLite outbox
// is explicitly unsupported. The unsupported refusal itself is covered by unit
// tests; these tests exercise the supported runtime.
func requireLinuxOutbox(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("streaming outbox is Linux-only; refusing elsewhere is covered by unit tests")
	}
}

func (f *e2eFixture) budgetFile(t *testing.T) string {
	t.Helper()
	budget := contracts.ResourceBudget{
		MaxBatchSize: 1000, MaxWorkers: 16, MaxRowBytes: 1 << 20, MaxColumns: 64,
		MaxTables: 8, MaxHistoryEntries: 512, MaxHistoryNameBytes: 4096,
		MaxReceiptPage: 64, MaxIssueCounts: 16, MaxQueue: 0,
		MaxProcessBytes: 1 << 30, MaxDiskBytes: 1 << 30,
	}
	raw, err := json.Marshal(budget)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, "budget.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	f.secrets = append(f.secrets, path)
	return path
}

func (f *e2eFixture) streamingApproveArgs(scopes ...string) []string {
	args := append([]string{"approve"}, f.planArgs(scopes...)...)
	args = append(args, f.review.reviewFlags()...)
	return append(args, "--reviewer-id", "reviewer@synthetic.example", "--streaming")
}

func (f *e2eFixture) streamingApplyArgs(budget string, scopes ...string) []string {
	args := append([]string{"mask"}, f.planArgs(scopes...)...)
	args = append(args, "--key-file", f.keyFile)
	args = append(args, f.review.applyFlags()...)
	args = append(args, "--approved", "--apply", "--streaming",
		"--budget-file", budget, "--run-id", e2eRunID, "--receipt-page-size", "8")
	return args
}

func (f *e2eFixture) initOutbox(t *testing.T) {
	t.Helper()
	f.run(t, 0, "init-outbox", "--target", f.target)
}

func TestE2EStreamingFullWorkflow(t *testing.T) {
	requireLinuxOutbox(t)
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}
	budget := f.budgetFile(t)
	// Preview must not create the outbox run, a history record or writes.
	before := f.targetDigest(t)
	f.run(t, 0, f.previewArgs(scopes...)...)
	if f.targetDigest(t) != before {
		t.Fatal("preview changed the target")
	}
	f.initOutbox(t)
	f.run(t, 0, f.streamingApproveArgs(scopes...)...)
	// Legacy approval of the same plan is irrelevant to the streaming path; the
	// streaming record is what authorizes it.
	applied := f.run(t, 0, f.streamingApplyArgs(budget, scopes...)...)
	for _, want := range []string{
		"streaming: protocol=m2-stream-v1 run=" + e2eRunID,
		"state=complete audit=consistent",
		"rows_committed=3",
		"batches_total=1 batches_committed=1 batches_failed=0 batches_unknown=0",
		"receipts page_size=8 pages=1 bounded=true",
		"budget_status=experimental",
	} {
		if !strings.Contains(applied, want) {
			t.Fatalf("streaming output missing %q: %q", want, applied)
		}
	}
	if f.targetDigest(t) == before {
		t.Fatal("streaming apply did not change the target")
	}
	if got := f.run(t, 0, f.validateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
		t.Fatalf("strict validation output=%q", got)
	}
	database, err := sql.Open("sqlite", "file:"+f.target+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var batches int
	if err := database.QueryRow(`SELECT COUNT(*) FROM maskriver_outbox_batch`).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if batches != 1 {
		t.Fatalf("outbox batches=%d", batches)
	}
}

func TestE2EStreamingRequiresExplicitApplyAndApproval(t *testing.T) {
	requireLinuxOutbox(t)
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}
	budget := f.budgetFile(t)
	f.initOutbox(t)
	before := f.targetDigest(t)
	// --streaming without --apply is refused and writes nothing.
	args := append([]string{"mask"}, f.planArgs(scopes...)...)
	args = append(args, "--key-file", f.keyFile, "--streaming", "--budget-file", budget)
	f.run(t, 2, args...)
	// --apply without a streaming approval is refused before any transaction.
	f.run(t, 1, f.streamingApplyArgs(budget, scopes...)...)
	if f.targetDigest(t) != before {
		t.Fatal("refused streaming runs wrote data")
	}
}

func TestE2EStreamingRejectsLegacyApproval(t *testing.T) {
	requireLinuxOutbox(t)
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}
	budget := f.budgetFile(t)
	f.initOutbox(t)
	f.run(t, 0, f.approveArgs(scopes...)...) // legacy m1 decision only
	before := f.targetDigest(t)
	f.run(t, 1, f.streamingApplyArgs(budget, scopes...)...)
	if f.targetDigest(t) != before {
		t.Fatal("legacy approval authorized the streaming path")
	}
}

func TestE2EStreamingRevokedAndMissingSchema(t *testing.T) {
	requireLinuxOutbox(t)
	t.Run("revoked", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		scopes := []string{"people.email"}
		budget := f.budgetFile(t)
		f.initOutbox(t)
		f.run(t, 0, f.streamingApproveArgs(scopes...)...)
		revoke := append([]string{"revoke"}, f.planArgs(scopes...)...)
		revoke = append(revoke, f.review.reviewFlags()...)
		revoke = append(revoke, "--streaming")
		f.run(t, 0, revoke...)
		before := f.targetDigest(t)
		f.run(t, 1, f.streamingApplyArgs(budget, scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("revoked approval wrote data")
		}
	})
	t.Run("missing schema", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		scopes := []string{"people.email"}
		budget := f.budgetFile(t)
		f.run(t, 0, f.streamingApproveArgs(scopes...)...)
		before := f.targetDigest(t)
		f.run(t, 1, f.streamingApplyArgs(budget, scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("apply without outbox schema wrote data")
		}
	})
	t.Run("wrong schema version", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		scopes := []string{"people.email"}
		budget := f.budgetFile(t)
		f.initOutbox(t)
		database, err := sql.Open("sqlite", "file:"+f.target+"?mode=rw")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE maskriver_outbox_schema SET version='maskriver-outbox-v0'`); err != nil {
			_ = database.Close()
			t.Fatal(err)
		}
		_ = database.Close()
		f.run(t, 0, f.streamingApproveArgs(scopes...)...)
		before := f.targetDigest(t)
		f.run(t, 1, f.streamingApplyArgs(budget, scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("apply with wrong schema version wrote data")
		}
	})
}

func TestE2EStreamingRefusesReservedBusinessScope(t *testing.T) {
	requireLinuxOutbox(t)
	f := newE2EFixture(t, e2eOneTable...)
	f.run(t, 1, f.previewArgs("maskriver_outbox_batch.id")...)
}

func TestE2EStreamingTenThousandRows(t *testing.T) {
	requireLinuxOutbox(t)
	if testing.Short() {
		t.Skip("scale test skipped in -short")
	}
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	var inserts []string
	for i := 0; i < 10000; i++ {
		inserts = append(inserts, "('row"+strconv.Itoa(i%64)+"@example.invalid')")
	}
	statements := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
		`INSERT INTO people (email) VALUES ` + strings.Join(inserts, ","),
	}
	f := newE2EFixtureAt(t, dir, source, target, statements...)
	scopes := []string{"people.email"}
	budget := f.budgetFile(t)
	f.run(t, 0, f.previewArgs(scopes...)...)
	f.initOutbox(t)
	f.run(t, 0, f.streamingApproveArgs(scopes...)...)
	applied := f.run(t, 0, f.streamingApplyArgs(budget, scopes...)...)
	if !strings.Contains(applied, "rows_committed=10000") {
		t.Fatalf("10k streaming summary=%q", applied)
	}
	if !strings.Contains(applied, "batches_unknown=0") || !strings.Contains(applied, "state=complete") {
		t.Fatalf("10k streaming state=%q", applied)
	}
}

// newE2EFixtureAt is newE2EFixture with explicit database paths.
func newE2EFixtureAt(t *testing.T, dir, source, target string, statements ...string) *e2eFixture {
	t.Helper()
	f := &e2eFixture{
		dir: dir, source: source, target: target,
		keyFile: filepath.Join(dir, "masking.key"),
	}
	cliExec(t, f.source, statements...)
	raw, err := os.ReadFile(f.source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.keyFile, []byte(strings.Repeat("K", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	f.review = newReviewProvisioning(t)
	f.secrets = []string{
		f.source, f.target, f.keyFile,
		f.review.historyDir, f.review.trustFile, f.review.reviewerKeyFile,
		strings.Repeat("K", 40),
	}
	return f
}
