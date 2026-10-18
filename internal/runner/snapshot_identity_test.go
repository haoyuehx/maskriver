package runner

// Regression tests for work package B2b-1, snapshot identity. SourceSnapshotID
// is the SHA-256 of the source's main .db file, so it is only an identity of
// the source when that file holds every committed row. A WAL source can keep
// committed rows in the -wal sidecar, which makes the digest stale; the runner
// refuses such a source instead of claiming a snapshot it can not prove.
// Every fixture is synthetic and lives in t.TempDir().

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// walSnapshotFixture builds a WAL-mode source plus an ordinary target copy and
// returns the still-open source writer. Three rows are checkpointed into the
// main file, so later uncheckpointed rows are the only difference.
func walSnapshotFixture(t *testing.T) (config.Runtime, contracts.Value, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Runtime{
		Source: filepath.Join(dir, "source.db"), Target: filepath.Join(dir, "target.db"),
		Table: "people", Column: "email",
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
	target, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = target.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = target.Exec(`INSERT INTO people VALUES (?, ?)`, i+1, fmt.Sprintf("t%d@example.invalid", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}

	source, err := sql.Open("sqlite", "file:"+cfg.Source+"?mode=rwc&_pragma=journal_mode(wal)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = source.Exec(`INSERT INTO people VALUES (?, ?)`, i+1, fmt.Sprintf("a%d@example.invalid", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	// Leave no frames behind, so every later row is unambiguously uncheckpointed.
	if _, err = source.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, key, source
}

// TestWALSourceIsRefusedBySnapshotIdentity is the primary B2b-1 regression. It
// first reproduces the hazard against real SQLite (the main .db file keeps the
// same bytes while a fresh read-only session already sees the new row), then
// requires the runner to refuse that source and to leave the target untouched.
func TestWALSourceIsRefusedBySnapshotIdentity(t *testing.T) {
	cfg, key, source := walSnapshotFixture(t)
	ctx := context.Background()

	mainBefore, err := fileDigest(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec(`INSERT INTO people VALUES (4, 'd@example.invalid')`); err != nil {
		t.Fatal(err)
	}
	mainAfter, err := fileDigest(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := os.Stat(cfg.Source + "-wal")
	if err != nil {
		t.Fatalf("wal sidecar: %v", err)
	}
	t.Logf("main .db sha256 before=%s after=%s equal=%t; -wal size=%d",
		mainBefore, mainAfter, mainBefore == mainAfter, sidecar.Size())
	if mainBefore != mainAfter {
		t.Fatal("fixture did not reproduce the hazard: the main .db file changed")
	}
	if sidecar.Size() == 0 {
		t.Fatal("fixture did not leave committed rows in a non-empty -wal sidecar")
	}
	fresh, err := sql.Open("sqlite", "file:"+cfg.Source+"?mode=ro")
	if err != nil {
		t.Fatalf("fresh read-only open: %v", err)
	}
	var visible int
	if err = fresh.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&visible); err != nil {
		t.Fatalf("fresh read-only count: %v", err)
	}
	if err = fresh.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("fresh read-only session sees %d rows, main-file digest describes 3", visible)
	if visible != 4 {
		t.Fatalf("fresh read-only session saw %d rows, want 4: the fixture must expose the uncheckpointed row", visible)
	}

	engine, err := Open(ctx, cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	before := acceptanceTreeDigest(t, cfg.Target)
	plan, err := engine.Plan(ctx)
	if !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("WAL source plan error=%v, want ErrIncomplete", err)
	}
	if plan.ID != "" || plan.SourceSnapshotID != "" || plan.ScanComplete {
		t.Fatalf("refused plan must stay empty: %+v", plan)
	}
	if strings.Contains(err.Error(), "example.invalid") {
		t.Fatalf("snapshot refusal leaked a value: %v", err)
	}
	spy := &writerSpy{}
	spyEngine(t, engine, spy)
	if _, err = engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}); err == nil {
		t.Fatal("apply accepted a refused plan")
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("WAL source apply used the Writer: %+v", spy)
	}
	if after := acceptanceTreeDigest(t, cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("WAL source refusal changed the target fixture tree")
	}

	// Closing the last connection checkpoints the WAL into the main file, which
	// is the proof that the digest taken while WAL was active was stale.
	if err = engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	mainCheckpointed, err := fileDigest(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("main .db sha256 after checkpoint on close=%s", mainCheckpointed)
	if mainCheckpointed == mainAfter {
		t.Fatal("checkpoint on close did not change the main .db file: the fixture can not show a stale digest")
	}
}

// TestWALSourceWithoutActiveWriterIsStillRefused pins the conservative choice:
// WAL is a persistent property of the file, and without an exclusive lock the
// main file can not be shown to be complete, so even a fully checkpointed WAL
// database is refused.
func TestWALSourceWithoutActiveWriterIsStillRefused(t *testing.T) {
	cfg, key, source := walSnapshotFixture(t)
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err = engine.Plan(context.Background()); !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("closed WAL source plan error=%v, want ErrIncomplete", err)
	}
}

// TestRollbackJournalSourcePlansAppliesAndValidates is the positive control: a
// quiescent rollback-journal source keeps today's behaviour, and its snapshot
// identity is exactly the main-file digest.
func TestRollbackJournalSourcePlansAppliesAndValidates(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	ctx := context.Background()
	want, err := fileDigest(e.cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	if want != digestOf(t, e.cfg.Source) {
		t.Fatal("fileDigest is not the main-file SHA-256")
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatalf("quiescent source plan: %v", err)
	}
	if plan.SourceSnapshotID != want {
		t.Fatalf("SourceSnapshotID=%s, want the main-file digest %s", plan.SourceSnapshotID, want)
	}
	report, err := e.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil || !report.Complete || len(report.Committed) != 1 || report.Committed[0].Rows != 2 {
		t.Fatalf("apply report=%+v error=%v", report, err)
	}
	validation, err := e.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil {
		t.Fatalf("strict validate: %v", err)
	}
	if !validation.Passed(true) {
		t.Fatalf("strict validate did not pass after apply: issues=%+v coverage=%+v",
			validation.Issues, validation.Coverage)
	}
}

// digestOf is the plain SHA-256 the plan digest is defined as, computed here so
// a change of hashing scheme in the runner can not make the assertion
// self-referential.
func digestOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
