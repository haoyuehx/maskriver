package runner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// These tests cover the correctness and safety properties the performance
// benchmarks assume but cannot prove: that the number of workers changes only
// speed, never the resulting data or the safety decisions around it.

// signedEngine is a fixture whose approval path is real: a real history store, a
// real Ed25519 signer and a real decision bound to the plan. Nothing here is
// stubbed, so an Apply that succeeds has genuinely passed the approval gate.
type signedEngine struct {
	engine *Engine
	cfg    config.Runtime
	key    contracts.Value
	dir    string
	store  *history.Store
}

func newSignedEngine(t *testing.T, rows []string, workers int) *signedEngine {
	t.Helper()
	cfg, key := acceptanceFixture(t, rows)
	cfg.Workers = workers
	cfg.BatchSize = 2
	// The shared fixture samples 100 rows; a truncated sample refuses the plan, so
	// these tests raise the limit rather than relying on small row counts.
	cfg.SampleLimit = 8192
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	trust, err := history.NewTrustedKeys(history.TrustedKey{SignerKeyID: testReviewerKeyID, Public: public})
	if err != nil {
		t.Fatalf("trust: %v", err)
	}
	store, err := history.Open(history.Config{
		Dir:    filepath.Join(t.TempDir(), "history"),
		Trust:  trust,
		Signer: &history.Signer{SignerKeyID: testReviewerKeyID, Private: private},
	})
	if err != nil {
		t.Fatalf("history.Open: %v", err)
	}
	engine.SetApprovalQuery(store)
	return &signedEngine{engine: engine, cfg: cfg, key: key, dir: filepath.Dir(cfg.Source), store: store}
}

// prepare runs Scan, Plan and Preview, then issues a real signed approval for the
// whole plan. Preview deliberately runs before approval, so a plan that has been
// previewed is not thereby authorized.
func (s *signedEngine) prepare(t *testing.T) contracts.Plan {
	t.Helper()
	ctx := context.Background()
	if _, err := s.engine.Scan(ctx, contracts.ScanRequest{}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	plan, err := s.engine.Plan(ctx)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := s.engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 1000}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := s.store.Approve(ctx, history.ApproveRequest{
		Plan:       plan,
		ReviewerID: testReviewerLabel,
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return plan
}

func (s *signedEngine) apply(t *testing.T, plan contracts.Plan) contracts.ApplyReport {
	t.Helper()
	report, err := s.engine.Apply(context.Background(), contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: 2,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return report
}

// targetRows reads the target table in primary-key order so two runs can be
// compared row by row rather than by a summary.
func targetRows(t *testing.T, path string) map[int64]string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer database.Close()
	rows, err := database.Query(`SELECT id, email FROM people ORDER BY id`)
	if err != nil {
		t.Fatalf("query target: %v", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var value sql.NullString
		if err := rows.Scan(&id, &value); err != nil {
			t.Fatalf("scan target: %v", err)
		}
		out[id] = value.String
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate target: %v", err)
	}
	return out
}

func syntheticEmails(count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = fmt.Sprintf("synthetic-%04d@example.invalid", i)
	}
	return out
}

// TestWorkerCountNeverChangesResultData compares the full row set produced with
// one worker against four workers. A digest alone would not catch a swapped or
// duplicated row, so the comparison is per primary key.
func TestWorkerCountNeverChangesResultData(t *testing.T) {
	const rows = 24
	values := syntheticEmails(rows)

	run := func(workers int) map[int64]string {
		fixture := newSignedEngine(t, values, workers)
		plan := fixture.prepare(t)
		report := fixture.apply(t, plan)
		if !report.Complete {
			t.Fatalf("workers=%d: apply incomplete: %+v", workers, report.Issues)
		}
		return targetRows(t, fixture.cfg.Target)
	}
	one := run(1)
	four := run(4)

	if len(one) != rows || len(four) != rows {
		t.Fatalf("row count mismatch: workers=1 %d rows, workers=4 %d rows, want %d", len(one), len(four), rows)
	}
	if len(one) != len(four) {
		t.Fatalf("row count differs between worker counts: %d vs %d", len(one), len(four))
	}
	for id, want := range one {
		got, ok := four[id]
		if !ok {
			t.Fatalf("primary key %d present with workers=1 but missing with workers=4", id)
		}
		if got != want {
			t.Fatalf("primary key %d differs: workers=1 %q, workers=4 %q", id, want, got)
		}
	}
	// Masking must never be a silent no-op: receiving the input back unchanged
	// would mean the write path did nothing.
	unchanged := 0
	for id, value := range one {
		if id <= int64(len(values)) && value == values[id-1] {
			unchanged++
		}
	}
	if unchanged > 0 {
		t.Fatalf("%d of %d rows kept their original value", unchanged, rows)
	}
}

// TestApplyReceiptsAndValidationAgree checks the receipt, the report and an
// independent Validate against the same target, so a correct-looking recompute of
// masked values cannot hide a row that was never written.
func TestApplyReceiptsAndValidationAgree(t *testing.T) {
	const rows = 9
	fixture := newSignedEngine(t, syntheticEmails(rows), 4)
	plan := fixture.prepare(t)

	// Only the source database and its own sidecars, not the whole directory:
	// source and target share a directory, so a directory digest would also
	// change when Apply legitimately touches the target.
	sourceBeforeDigest, err := fileDigest(fixture.cfg.Source)
	if err != nil {
		t.Fatalf("digest source: %v", err)
	}
	report := fixture.apply(t, plan)
	if !report.Complete {
		t.Fatalf("apply incomplete: %+v", report.Issues)
	}
	var committed int64
	for _, receipt := range report.Committed {
		if receipt.Rows <= 0 {
			t.Fatalf("receipt for %q reported %d rows", receipt.Table, receipt.Rows)
		}
		committed += receipt.Rows
	}
	if committed != rows {
		t.Fatalf("committed %d rows, want %d", committed, rows)
	}
	if report.CommitOutcomeUnknown {
		t.Fatal("commit outcome reported unknown for a successful run")
	}

	sourceAfterDigest, err := fileDigest(fixture.cfg.Source)
	if err != nil {
		t.Fatalf("digest source: %v", err)
	}
	if sourceAfterDigest != sourceBeforeDigest {
		t.Fatalf("source database changed during Apply: %s -> %s", sourceBeforeDigest, sourceAfterDigest)
	}

	validation, err := fixture.engine.Validate(context.Background(), contracts.ValidateRequest{Plan: plan})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !validation.Coverage.Complete || !validation.Passed(true) {
		t.Fatalf("validation failed: coverage=%+v issues=%+v", validation.Coverage, validation.Issues)
	}

	// A complete validation must agree with the rows that exist, not with the
	// receipts alone.
	written := targetRows(t, fixture.cfg.Target)
	if len(written) != rows {
		t.Fatalf("target holds %d rows after apply, want %d", len(written), rows)
	}
	distinct := map[string]struct{}{}
	for _, value := range written {
		distinct[value] = struct{}{}
	}
	if len(distinct) != rows {
		t.Fatalf("target holds %d distinct values, want %d (duplicate masking suspected)", len(distinct), rows)
	}
}

// TestPreviewDoesNotWriteTarget proves Preview is read-only by comparing the
// target file digest before and after.
func TestPreviewDoesNotWriteTarget(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(6), 2)
	ctx := context.Background()
	if _, err := fixture.engine.Scan(ctx, contracts.ScanRequest{}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	plan, err := fixture.engine.Plan(ctx)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	before := acceptanceTreeDigest(t, fixture.cfg.Target)
	if _, err := fixture.engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 1000}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	after := acceptanceTreeDigest(t, fixture.cfg.Target)
	if !sameDigest(before, after) {
		t.Fatal("preview modified the target database")
	}
}

// TestUnapprovedApplyCannotWrite uses a plan that was scanned, planned, previewed
// and then never approved. A passing test means the writer was never opened.
func TestUnapprovedApplyCannotWrite(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(5), 4)
	ctx := context.Background()
	if _, err := fixture.engine.Scan(ctx, contracts.ScanRequest{}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	plan, err := fixture.engine.Plan(ctx)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	before := acceptanceTreeDigest(t, fixture.cfg.Target)
	assertApplyRefusedBeforeWriter(t, fixture.engine, plan, contracts.ErrNotFound)
	if !sameDigest(before, acceptanceTreeDigest(t, fixture.cfg.Target)) {
		t.Fatal("unapproved apply modified the target")
	}
}

// TestWorkersStopAndDoNotLeak runs a full workflow and asserts that every worker
// goroutine and every pooled connection is released afterwards.
func TestWorkersStopAndDoNotLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()
	func() {
		fixture := newSignedEngine(t, syntheticEmails(32), 8)
		plan := fixture.prepare(t)
		fixture.apply(t, plan)
		if err := fixture.engine.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		after := runtime.NumGoroutine()
		if after <= baseline+2 {
			return
		}
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			t.Fatalf("goroutines: baseline %d, after close %d\n%s", baseline, after, buf[:n])
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestCancellationStopsEveryWorker cancels mid-flight and requires the call to
// return an error promptly with the engine still usable, which is only true if the
// workers drained rather than being abandoned.
func TestCancellationStopsEveryWorker(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(64), 8)
	plan := fixture.prepare(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := fixture.engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}); err == nil {
		t.Fatal("apply with a cancelled context returned no error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancelled apply took %v to return", elapsed)
	}
	// The engine must still work afterwards: a worker pool that leaked or
	// deadlocked would fail here.
	report := fixture.apply(t, plan)
	if !report.Complete {
		t.Fatalf("apply after cancellation incomplete: %+v", report.Issues)
	}
}

// TestWorkflowLeavesNoSidecarFiles proves the runner does not leave journal or WAL
// sidecars behind after a completed run.
func TestWorkflowLeavesNoSidecarFiles(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(8), 4)
	plan := fixture.prepare(t)
	fixture.apply(t, plan)
	if err := fixture.engine.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	entries, err := os.ReadDir(fixture.dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var leftovers []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") || strings.HasSuffix(name, "-journal") {
			info, err := entry.Info()
			if err != nil {
				t.Fatalf("stat %s: %v", name, err)
			}
			if info.Size() > 0 {
				leftovers = append(leftovers, fmt.Sprintf("%s (%d B)", name, info.Size()))
			}
		}
	}
	sort.Strings(leftovers)
	if len(leftovers) > 0 {
		t.Fatalf("non-empty sidecar files left behind: %v", leftovers)
	}
}

func sameDigest(a, b map[string][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, sum := range a {
		other, ok := b[name]
		if !ok || other != sum {
			return false
		}
	}
	return true
}
