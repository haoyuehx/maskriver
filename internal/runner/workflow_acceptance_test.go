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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// acceptanceFixture builds the synthetic SQLite source/target pair and the
// matching Runtime/Key used by the workflow tests.
func acceptanceFixture(t *testing.T, rows []string) (config.Runtime, contracts.Value) {
	t.Helper()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
			t.Fatal(err)
		}
		for i, email := range rows {
			if email == "<NULL>" {
				_, err = database.Exec(`INSERT INTO people VALUES (?, NULL)`, i+1)
			} else {
				_, err = database.Exec(`INSERT INTO people VALUES (?, ?)`, i+1, email)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if err = database.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "key.bin"), []byte(strings.Repeat("k", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("synthetic sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	key, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	cfg := config.Runtime{Source: source, Target: target, Table: "people", Column: "email", Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic", Approved: true, MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 2, Workers: 1}
	return cfg, key
}

func acceptanceEngine(t *testing.T, rows []string) (*Engine, func()) {
	t.Helper()
	cfg, key := acceptanceFixture(t, rows)
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	useTestApproval(engine)
	cleanup := func() { _ = engine.Close() }
	t.Cleanup(cleanup)
	return engine, cleanup
}

type writerSpy struct {
	contracts.Writer
	opens, begins, updates, commits int
	rollbacks                       int
	beginErr, updateErr, commitErr  error
	beginErrAt, updateErrAt         int
	commitErrAt                     int
	commitThenErr                   bool
	forcedRows                      *int64
	cancel                          context.CancelFunc
	cancelAt                        int
	closes                          int
	rollbackDeadline                bool
	rollbackContextErr              error
	rollbackDeadlineRemaining       time.Duration
	// begun records the table of every Begin call in call order, which is the
	// instrumentation a test uses to prove execution really reached a later
	// table or batch instead of refusing earlier.
	begun []contracts.TableRef
}

func (w *writerSpy) Begin(ctx context.Context, p contracts.TablePlan) (contracts.WriteTx, error) {
	w.begins++
	w.begun = append(w.begun, p.Table)
	if w.beginErr != nil && (w.beginErrAt == 0 || w.begins == w.beginErrAt) {
		return nil, w.beginErr
	}
	tx, e := w.Writer.Begin(ctx, p)
	if e != nil {
		return nil, e
	}
	return &txSpy{WriteTx: tx, spy: w}, nil
}

func (w *writerSpy) Close() error {
	w.closes++
	return w.Writer.Close()
}

type txSpy struct {
	contracts.WriteTx
	spy *writerSpy
}

func (tx *txSpy) Update(c context.Context, changes []contracts.RowChange) (int64, error) {
	tx.spy.updates++
	if tx.spy.updateErr != nil && (tx.spy.updateErrAt == 0 || tx.spy.updates == tx.spy.updateErrAt) {
		return 0, tx.spy.updateErr
	}
	if tx.spy.forcedRows != nil {
		return *tx.spy.forcedRows, nil
	}
	n, err := tx.WriteTx.Update(c, changes)
	if tx.spy.cancel != nil && (tx.spy.cancelAt == 0 || tx.spy.updates == tx.spy.cancelAt) {
		tx.spy.cancel()
	}
	return n, err
}
func (tx *txSpy) Commit(c context.Context) error {
	tx.spy.commits++
	if err := c.Err(); err != nil {
		return err
	}
	if tx.spy.commitErr != nil && (tx.spy.commitErrAt == 0 || tx.spy.commits == tx.spy.commitErrAt) {
		if tx.spy.commitThenErr {
			if err := tx.WriteTx.Commit(c); err != nil {
				return err
			}
		}
		return tx.spy.commitErr
	}
	return tx.WriteTx.Commit(c)
}
func (tx *txSpy) Rollback(c context.Context) error {
	tx.spy.rollbacks++
	tx.spy.rollbackContextErr = c.Err()
	deadline, ok := c.Deadline()
	tx.spy.rollbackDeadline = ok
	if ok {
		tx.spy.rollbackDeadlineRemaining = time.Until(deadline)
	}
	return tx.WriteTx.Rollback(c)
}

func spyEngine(t *testing.T, e *Engine, spy *writerSpy) {
	t.Helper()
	e.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		spy.opens++
		w, err := db.OpenWriter(ctx, opts)
		if err == nil {
			spy.Writer = w
		}
		return spy, err
	}
}

func TestApplyChecksConfigApprovedBeforeOpeningWriter(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	e.cfg.Approved = false
	_, err = e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Apply error=%v, want ErrUnsafe", err)
	}
	if spy.Writer != nil || spy.opens != 0 || spy.begins != 0 {
		t.Fatalf("unauthorized apply opened writer: factory=%d writer=%v begins=%d", spy.opens, spy.Writer != nil, spy.begins)
	}
}

func TestApplyRequiresExplicitIntentBeforeOpeningWriter(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	_, err = e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, BatchSize: 1})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Apply error=%v, want ErrUnsafe", err)
	}
	if spy.Writer != nil || spy.opens != 0 || spy.begins != 0 {
		t.Fatal("missing explicit intent opened Writer")
	}
}

func TestApplyPreflightFailuresDoNotOpenWriter(t *testing.T) {
	for _, mode := range []string{"tampered plan", "preview mismatch"} {
		t.Run(mode, func(t *testing.T) {
			e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
			plan, err := e.Plan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "tampered plan" {
				plan.ID = "tampered"
			} else {
				writer, err := sql.Open("sqlite", "file:"+e.cfg.Target+"?mode=rw")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = writer.Exec(`UPDATE people SET email=? WHERE id=1`, "other@example.invalid"); err != nil {
					t.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
			spy := &writerSpy{}
			spyEngine(t, e, spy)
			_, err = e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
			if err == nil {
				t.Fatal("preflight failure unexpectedly succeeded")
			}
			if spy.Writer != nil || spy.opens != 0 || spy.begins != 0 {
				t.Fatal("preflight failure opened Writer")
			}
		})
	}
}

func TestApplySkipsNullChangesAcrossBatches(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "<NULL>", "b@example.invalid", "<NULL>", "c@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	before := acceptanceTreeDigest(t, e.cfg.Source)
	preview, err := e.Preview(context.Background(), contracts.PreviewRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if preview.RowsRead != 5 {
		t.Fatalf("preview rows=%d, want all 5", preview.RowsRead)
	}
	if strings.Contains(fmt.Sprintf("%+v", preview), "a@example.invalid") {
		t.Fatal("Preview leaked a synthetic sensitive value")
	}
	if spy.Writer != nil || spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatal("Preview used writer capability")
	}
	if after := acceptanceTreeDigest(t, e.cfg.Source); !reflect.DeepEqual(after, before) {
		t.Fatal("Preview changed a file in the fixture directory")
	}
	spy = &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil {
		t.Fatalf("apply failed with nullable rows: %v", err)
	}
	if !report.Complete || len(report.Committed) != 3 || report.Committed[0].Rows != 1 || report.Committed[1].Rows != 1 || report.Committed[2].Rows != 1 {
		t.Fatalf("unexpected receipts: %+v", report)
	}
	if spy.updates != 3 {
		t.Fatalf("updates=%d, want 3 non-NULL batches", spy.updates)
	}
	committedRows := int64(0)
	for _, receipt := range report.Committed {
		committedRows += receipt.Rows
	}
	if committedRows != 3 {
		t.Fatalf("receipt rows=%d, want 3 actual updates", committedRows)
	}
	reader, err := sql.Open("sqlite", "file:"+e.cfg.Target+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := reader.Query(`SELECT id, email FROM people ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	nulls, changed := 0, 0
	for rows.Next() {
		var id int
		var email sql.NullString
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatal(err)
		}
		if !email.Valid {
			nulls++
			continue
		}
		original := map[int]string{1: "a@example.invalid", 3: "b@example.invalid", 5: "c@example.invalid"}[id]
		if email.String != original {
			changed++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if nulls != 2 || changed != 3 {
		t.Fatalf("SQLite readback NULLs=%d changed=%d; want 2/3", nulls, changed)
	}
	if strings.Contains(fmt.Sprintf("%+v", report), "a@example.invalid") {
		t.Fatal("Apply report leaked a synthetic sensitive value")
	}
}

func acceptanceTreeDigest(t *testing.T, databasePath string) map[string][32]byte {
	t.Helper()
	dir := filepath.Dir(databasePath)
	paths := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	result := make(map[string][32]byte, len(paths))
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[filepath.Base(path)] = sha256.Sum256(b)
	}
	return result
}

// TestPlanRefusesAllNullColumnWithoutWriteTransaction replaces the previous
// "all NULL apply completes" case. A column with no distinct non-NULL value
// yields UNKNOWN evidence, and Plan now refuses it instead of writing nothing
// under a plan that claims ScanComplete. The no-side-effect assertion is kept.
func TestPlanRefusesAllNullColumnWithoutWriteTransaction(t *testing.T) {
	nulls := make([]string, 130)
	for i := range nulls {
		nulls[i] = "<NULL>"
	}
	e, _ := acceptanceEngine(t, nulls)
	before := acceptanceTreeDigest(t, e.cfg.Target)
	plan, err := e.Plan(context.Background())
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("all-NULL column plan error=%v, want ErrUnsafe", err)
	}
	if plan.ID != "" || plan.ScanComplete {
		t.Fatalf("refused plan must be empty: %+v", plan)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	// The refusal must survive an explicit apply attempt, not just Plan.
	if _, applyErr := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}); applyErr == nil {
		t.Fatal("apply with a refused plan must fail")
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("refused plan performed writes: %+v", spy)
	}
	after := acceptanceTreeDigest(t, e.cfg.Target)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("refused plan changed the target fixture tree")
	}
}

func TestApplyRollsBackBeginBatchFailuresAndCancellation(t *testing.T) {
	tests := []struct {
		name                                   string
		configure                              func(*writerSpy, context.CancelFunc)
		wantBegins, wantUpdates, wantRollbacks int
	}{
		{"begin failure", func(s *writerSpy, _ context.CancelFunc) { s.beginErr = contracts.ErrDatabase }, 1, 0, 0},
		{"update failure", func(s *writerSpy, _ context.CancelFunc) { s.updateErr = contracts.ErrDatabase }, 1, 1, 1},
		{"affected row mismatch", func(s *writerSpy, _ context.CancelFunc) { zero := int64(0); s.forcedRows = &zero }, 1, 1, 1},
		{"cancel before commit", func(s *writerSpy, cancel context.CancelFunc) { s.cancel = cancel }, 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
			plan, err := e.Plan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			spy := &writerSpy{}
			tt.configure(spy, cancel)
			spyEngine(t, e, spy)
			report, err := e.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
			if err == nil || report.Complete || len(report.Committed) != 0 {
				t.Fatalf("failure path report=%+v err=%v", report, err)
			}
			if spy.begins != tt.wantBegins || spy.updates != tt.wantUpdates || spy.rollbacks != tt.wantRollbacks {
				t.Fatalf("spy begin/update/rollback=%d/%d/%d", spy.begins, spy.updates, spy.rollbacks)
			}
			if spy.closes != 1 {
				t.Fatalf("writer close calls=%d, want 1", spy.closes)
			}
			if tt.wantRollbacks != 0 && (!spy.rollbackDeadline || spy.rollbackContextErr != nil || spy.rollbackDeadlineRemaining <= 0) {
				t.Fatalf("rollback context deadline=%v err=%v remaining=%s", spy.rollbackDeadline, spy.rollbackContextErr, spy.rollbackDeadlineRemaining)
			}
			if tt.name == "cancel before commit" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel error=%v, want context.Canceled", err)
			}
			if got := readTargetEmails(t, e.cfg.Target); got[1] != "a@example.invalid" {
				t.Fatalf("fault case changed durable target value: %q", got[1])
			}
		})
	}
}

func TestApplyWrappedCommitUnknownIsNotReportedAsRolledBack(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{commitErr: fmt.Errorf("driver status: %w", contracts.ErrCommitUnknown), commitErrAt: 2}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrCommitUnknown) || !report.CommitOutcomeUnknown || report.Complete {
		t.Fatalf("unexpected unknown commit result report=%+v error=%v", report, err)
	}
	if len(report.Committed) != 1 || spy.rollbacks != 0 || spy.commits != 2 || spy.closes != 1 {
		t.Fatalf("unknown commit must not rollback/retry: rollbacks=%d commits=%d", spy.rollbacks, spy.commits)
	}
}

func TestApplyWrappedCommitUnknownAfterDurableCommitReportsUnknown(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{
		commitErr:   fmt.Errorf("commit acknowledgment lost: %w", contracts.ErrCommitUnknown),
		commitErrAt: 2, commitThenErr: true,
	}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrCommitUnknown) || !report.CommitOutcomeUnknown || report.Complete {
		t.Fatalf("unexpected durable-unknown result report=%+v err=%v", report, err)
	}
	if len(report.Committed) != 1 || report.Committed[0].Rows != 1 || spy.commits != 2 || spy.rollbacks != 0 || spy.closes != 1 {
		t.Fatalf("unknown batch was retried/claimed committed/rolled back: report=%+v commits=%d rollbacks=%d closes=%d", report, spy.commits, spy.rollbacks, spy.closes)
	}
	got := readTargetEmails(t, e.cfg.Target)
	if got[1] == "a@example.invalid" || got[2] == "b@example.invalid" {
		t.Fatalf("expected both real SQLite commits persisted despite second unknown report; first=%q second=%q", got[1], got[2])
	}
}

func TestApplyRetainsReceiptForEarlierCommittedBatch(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &writerSpy{commitErr: contracts.ErrDatabase, commitErrAt: 2}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if err == nil || report.Complete || report.CommitOutcomeUnknown {
		t.Fatalf("unexpected partial failure report=%+v err=%v", report, err)
	}
	if len(report.Committed) != 1 || report.Committed[0].Rows != 1 || report.Committed[0].Ordinal != 1 {
		t.Fatalf("earlier committed batch receipt lost: %+v", report.Committed)
	}
	if spy.commits != 2 || spy.rollbacks != 1 {
		t.Fatalf("later failed batch handling commits=%d rollbacks=%d", spy.commits, spy.rollbacks)
	}
	if spy.closes != 1 {
		t.Fatalf("writer close calls=%d, want 1", spy.closes)
	}
	got := readTargetEmails(t, e.cfg.Target)
	if got[1] == "a@example.invalid" || got[2] != "b@example.invalid" {
		t.Fatalf("expected first row committed and second unchanged; got ids 1=%q 2=%q", got[1], got[2])
	}
}

func readTargetEmails(t *testing.T, path string) map[int]string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`SELECT id, email FROM people ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[int]string)
	for rows.Next() {
		var id int
		var email sql.NullString
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatal(err)
		}
		if email.Valid {
			result[id] = email.String
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
