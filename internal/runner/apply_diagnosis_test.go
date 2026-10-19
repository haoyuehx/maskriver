package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file diagnoses where Apply spends its time. It measures and reports; it
// changes no batch size, no transaction policy, no journal mode and no worker
// count.

type applyDiag struct {
	rows              int64
	receipts          int
	batches           int
	statements        int
	updates           int
	beginAttempts     int
	beginSucceeded    int
	beginFailed       int
	commitAttempts    int
	commitSucceeded   int
	commitFailed      int
	commitUnknown     int
	rollbackAttempts  int
	rollbackSucceeded int
	rollbackFailed    int
	beginTime         time.Duration
	commitTime        time.Duration
	rollbackTime      time.Duration
	updateTime        time.Duration
	total             time.Duration
	perBatch          time.Duration
	perRow            time.Duration
}

// spyBatchWriter counts transactions and statements without changing what the
// writer does: every call is delegated to the real writer.
type spyBatchWriter struct {
	contracts.Writer
	mu                   sync.Mutex
	beginAttempts        int
	beginSucceeded       int
	beginFailed          int
	commitAttempts       int
	commitSucceeded      int
	commitFailed         int
	commitUnknown        int
	unknownAfterCommit   bool
	failUpdateErr        error
	rollbackAttempts     int
	rollbackSucceeded    int
	rollbackFailed       int
	beginTime            time.Duration
	commitTime           time.Duration
	rollbackTime         time.Duration
	updateTime           time.Duration
	batchWindows         []time.Duration
	batchStatementCounts []int
	batchUpdateCounts    []int
	batchOtherCounts     []int
	batchBeginDriverNS   []int64
	batchCommitDriverNS  []int64
	batchUpdateExecNS    []int64
	batchOtherSQLCallNS  []int64
	sqlSpy               *db.SQLSpy
}

func (s *spyBatchWriter) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	started := time.Now()
	statementStart, eventStart := 0, 0
	if s.sqlSpy != nil {
		statementStart = len(s.sqlSpy.Statements())
		eventStart = len(s.sqlSpy.Events())
	}
	tx, err := s.Writer.Begin(ctx, plan)
	s.mu.Lock()
	s.beginAttempts++
	s.beginTime += time.Since(started)
	if err != nil {
		s.beginFailed++
	} else {
		s.beginSucceeded++
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &spyTx{WriteTx: tx, parent: s, unknownAfterCommit: s.unknownAfterCommit, failUpdateErr: s.failUpdateErr, batchStart: started, statementStart: statementStart, eventStart: eventStart}, nil
}

type spyTx struct {
	contracts.WriteTx
	parent             *spyBatchWriter
	unknownAfterCommit bool
	failUpdateErr      error
	batchStart         time.Time
	statementStart     int
	eventStart         int
}

func (t *spyTx) Update(ctx context.Context, changes []contracts.RowChange) (int64, error) {
	started := time.Now()
	if t.failUpdateErr != nil {
		t.parent.mu.Lock()
		t.parent.updateTime += time.Since(started)
		t.parent.mu.Unlock()
		return 0, t.failUpdateErr
	}
	n, err := t.WriteTx.Update(ctx, changes)
	t.parent.mu.Lock()
	t.parent.updateTime += time.Since(started)
	t.parent.mu.Unlock()
	return n, err
}

func (t *spyTx) Commit(ctx context.Context) error {
	started := time.Now()
	err := t.WriteTx.Commit(ctx)
	if err == nil && t.unknownAfterCommit {
		err = contracts.ErrCommitUnknown
	}
	batchElapsed := time.Since(t.batchStart)
	var batchStatements, batchUpdates, batchOther int
	var batchBeginNS, batchCommitNS, batchUpdateNS, batchOtherNS int64
	if t.parent.sqlSpy != nil {
		all := t.parent.sqlSpy.Statements()
		if t.statementStart <= len(all) {
			batch := all[t.statementStart:]
			batchStatements = len(batch)
			for _, statement := range batch {
				if statement.Verb == "UPDATE" {
					batchUpdates++
				}
			}
			batchOther = batchStatements - batchUpdates - 2
		}
		events := t.parent.sqlSpy.Events()
		if t.eventStart <= len(events) {
			for _, event := range events[t.eventStart:] {
				ns := event.Duration.Nanoseconds()
				switch event.Verb {
				case "BEGIN":
					batchBeginNS += ns
				case "COMMIT":
					batchCommitNS += ns
				case "UPDATE":
					batchUpdateNS += ns
				case "ROLLBACK":
				default:
					batchOtherNS += ns
				}
			}
		}
	}
	t.parent.mu.Lock()
	t.parent.commitAttempts++
	t.parent.commitTime += time.Since(started)
	if err != nil {
		t.parent.commitFailed++
		if errors.Is(err, contracts.ErrCommitUnknown) {
			t.parent.commitUnknown++
		}
	} else {
		t.parent.commitSucceeded++
	}
	t.parent.batchWindows = append(t.parent.batchWindows, batchElapsed)
	t.parent.batchStatementCounts = append(t.parent.batchStatementCounts, batchStatements)
	t.parent.batchUpdateCounts = append(t.parent.batchUpdateCounts, batchUpdates)
	t.parent.batchOtherCounts = append(t.parent.batchOtherCounts, batchOther)
	t.parent.batchBeginDriverNS = append(t.parent.batchBeginDriverNS, batchBeginNS)
	t.parent.batchCommitDriverNS = append(t.parent.batchCommitDriverNS, batchCommitNS)
	t.parent.batchUpdateExecNS = append(t.parent.batchUpdateExecNS, batchUpdateNS)
	t.parent.batchOtherSQLCallNS = append(t.parent.batchOtherSQLCallNS, batchOtherNS)
	t.parent.mu.Unlock()
	return err
}

func (t *spyTx) Rollback(ctx context.Context) error {
	started := time.Now()
	err := t.WriteTx.Rollback(ctx)
	t.parent.mu.Lock()
	t.parent.rollbackAttempts++
	t.parent.rollbackTime += time.Since(started)
	if err != nil {
		t.parent.rollbackFailed++
	} else {
		t.parent.rollbackSucceeded++
	}
	t.parent.mu.Unlock()
	return err
}

func diagnoseApply(t *testing.T, rows int, batchSize int, workers int) applyDiag {
	t.Helper()
	values := syntheticEmails(rows)
	fixture := newSignedEngine(t, values, workers)
	fixture.cfg.BatchSize = batchSize
	spy := db.NewSQLSpy()
	var observed *spyBatchWriter
	fixture.engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		writer, err := spy.OpenWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		observed = &spyBatchWriter{Writer: writer, sqlSpy: spy}
		return observed, nil
	}
	plan := fixture.prepare(t)

	start := time.Now()
	report := fixture.applyBatched(t, plan, batchSize)
	total := time.Since(start)
	if !report.Complete {
		t.Fatalf("apply incomplete: %+v", report.Issues)
	}
	var written int64
	for _, receipt := range report.Committed {
		written += receipt.Rows
	}
	statements := spy.Statements()
	updates := 0
	for _, statement := range statements {
		if statement.Verb == "UPDATE" {
			updates++
		}
	}
	if observed == nil {
		t.Fatal("writer was not opened")
	}
	observed.mu.Lock()
	beginAttempts, beginSucceeded, beginFailed := observed.beginAttempts, observed.beginSucceeded, observed.beginFailed
	commitAttempts, commitSucceeded, commitFailed := observed.commitAttempts, observed.commitSucceeded, observed.commitFailed
	commitUnknown := observed.commitUnknown
	rollbackAttempts, rollbackSucceeded, rollbackFailed := observed.rollbackAttempts, observed.rollbackSucceeded, observed.rollbackFailed
	beginTime, commitTime := observed.beginTime, observed.commitTime
	rollbackTime, updateTime := observed.rollbackTime, observed.updateTime
	observed.mu.Unlock()
	return applyDiag{
		rows: written, receipts: len(report.Committed), batches: len(report.Committed),
		statements: len(statements), updates: updates,
		beginAttempts: beginAttempts, beginSucceeded: beginSucceeded, beginFailed: beginFailed,
		commitAttempts: commitAttempts, commitSucceeded: commitSucceeded, commitFailed: commitFailed, commitUnknown: commitUnknown,
		rollbackAttempts: rollbackAttempts, rollbackSucceeded: rollbackSucceeded, rollbackFailed: rollbackFailed,
		beginTime: beginTime, commitTime: commitTime, rollbackTime: rollbackTime, updateTime: updateTime,
		total:    total,
		perBatch: total / time.Duration(len(report.Committed)),
		perRow:   total / time.Duration(written),
	}
}

func TestApplyDiagnosisCountsRollbackOnUpdateFailure(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(1), 1)
	injected := errors.New("synthetic update failure")
	spy := db.NewSQLSpy()
	var observed *spyBatchWriter
	fixture.engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		writer, err := spy.OpenWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		observed = &spyBatchWriter{Writer: writer, sqlSpy: spy, failUpdateErr: injected}
		return observed, nil
	}
	plan := fixture.prepare(t)
	report, err := fixture.engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, injected) {
		t.Fatalf("Apply error=%v, want injected Update error", err)
	}
	if report.Complete || len(report.Committed) != 0 || report.CommitOutcomeUnknown {
		t.Fatalf("failed update produced success/receipt: %+v", report)
	}
	observed.mu.Lock()
	beginA, beginS, beginF := observed.beginAttempts, observed.beginSucceeded, observed.beginFailed
	commitA, commitS, commitF := observed.commitAttempts, observed.commitSucceeded, observed.commitFailed
	rollbackA, rollbackS, rollbackF := observed.rollbackAttempts, observed.rollbackSucceeded, observed.rollbackFailed
	observed.mu.Unlock()
	if beginA != 1 || beginS != 1 || beginF != 0 || commitA != 0 || commitS != 0 || commitF != 0 || rollbackA != 1 || rollbackS != 1 || rollbackF != 0 {
		t.Fatalf("unexpected begin/commit/rollback counts: begin %d/%d/%d commit %d/%d/%d rollback %d/%d/%d", beginA, beginS, beginF, commitA, commitS, commitF, rollbackA, rollbackS, rollbackF)
	}
	rows := targetRows(t, fixture.cfg.Target)
	if rows[1] != syntheticEmails(1)[0] {
		t.Fatalf("failed update changed target: %v", rows)
	}
}

func TestApplyDiagnosisDoesNotReportUnknownCommitAsSuccess(t *testing.T) {
	fixture := newSignedEngine(t, syntheticEmails(1), 1)
	spy := db.NewSQLSpy()
	var observed *spyBatchWriter
	fixture.engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		writer, err := spy.OpenWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		observed = &spyBatchWriter{Writer: writer, unknownAfterCommit: true, sqlSpy: spy}
		return observed, nil
	}
	plan := fixture.prepare(t)
	report, err := fixture.engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrCommitUnknown) {
		t.Fatalf("Apply error=%v, want ErrCommitUnknown", err)
	}
	if !report.CommitOutcomeUnknown || report.Complete || len(report.Committed) != 0 {
		t.Fatalf("unknown commit misreported: %+v", report)
	}
	observed.mu.Lock()
	attempts, succeeded, failed, unknown := observed.commitAttempts, observed.commitSucceeded, observed.commitFailed, observed.commitUnknown
	observed.mu.Unlock()
	if attempts != 1 || succeeded != 0 || failed != 1 || unknown != 1 {
		t.Fatalf("commit attempts/success/fail/unknown=%d/%d/%d/%d, want 1/0/1/1", attempts, succeeded, failed, unknown)
	}
	rows := targetRows(t, fixture.cfg.Target)
	if len(rows) != 1 || rows[1] == syntheticEmails(1)[0] {
		t.Fatalf("fault injection expected durable underlying commit but got target=%v", rows)
	}
}

// applyBatched is apply with an explicit batch size, so the diagnosis can vary it
// without changing any default.
func (s *signedEngine) applyBatched(t *testing.T, plan contracts.Plan, batchSize int) contracts.ApplyReport {
	t.Helper()
	report, err := s.engine.Apply(context.Background(), contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: batchSize,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return report
}

// TestApplyPathDiagnosis reports statement, transaction and per-row costs for a
// few batch sizes. It is a measurement, so it asserts only invariants that would
// make the numbers meaningless if violated.
func TestApplyPathDiagnosis(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnosis runs several applies")
	}
	const rows = 256
	for _, batchSize := range []int{16, 64, 256} {
		diag := diagnoseApply(t, rows, batchSize, 4)
		t.Logf("batch=%d rows=%d receipts=%d statements=%d updates=%d begin=%d/%d failed=%d commit=%d/%d failed=%d unknown=%d rollback=%d/%d failed=%d begin_ns=%d update_ns=%d commit_ns=%d rollback_ns=%d total=%v perBatch=%v perRow=%v",
			batchSize, diag.rows, diag.receipts, diag.statements, diag.updates,
			diag.beginAttempts, diag.beginSucceeded, diag.beginFailed, diag.commitAttempts, diag.commitSucceeded, diag.commitFailed, diag.commitUnknown,
			diag.rollbackAttempts, diag.rollbackSucceeded, diag.rollbackFailed, diag.beginTime.Nanoseconds(), diag.updateTime.Nanoseconds(),
			diag.commitTime.Nanoseconds(), diag.rollbackTime.Nanoseconds(), diag.total.Round(time.Microsecond), diag.perBatch.Round(time.Microsecond), diag.perRow.Round(time.Microsecond))
		if diag.rows != rows {
			t.Fatalf("batch=%d wrote %d rows, want %d", batchSize, diag.rows, rows)
		}
		wantBatches := (rows + batchSize - 1) / batchSize
		if diag.beginAttempts != wantBatches || diag.beginSucceeded != wantBatches || diag.beginFailed != 0 {
			t.Fatalf("batch=%d begin attempts/success/fail=%d/%d/%d want %d/%d/0", batchSize, diag.beginAttempts, diag.beginSucceeded, diag.beginFailed, wantBatches, wantBatches)
		}
		if diag.commitAttempts != wantBatches || diag.commitSucceeded != wantBatches || diag.commitFailed != 0 {
			t.Fatalf("batch=%d commit attempts/success/fail=%d/%d/%d want %d/%d/0", batchSize, diag.commitAttempts, diag.commitSucceeded, diag.commitFailed, wantBatches, wantBatches)
		}
		if diag.rollbackAttempts != 0 || diag.rollbackFailed != 0 {
			t.Fatalf("batch=%d rollback attempts/fail=%d/%d, want 0/0", batchSize, diag.rollbackAttempts, diag.rollbackFailed)
		}
		if diag.receipts != wantBatches {
			t.Fatalf("batch=%d produced %d receipts, want %d", batchSize, diag.receipts, (rows+batchSize-1)/batchSize)
		}
		// One UPDATE per row is what the current write path does; recording it here
		// is what makes "cost is per row, not per row-set" checkable.
		if diag.updates != rows {
			t.Fatalf("batch=%d issued %d UPDATE statements, want one per row (%d)", batchSize, diag.updates, rows)
		}
	}

	// Does the batch size change throughput at all? Compare the extremes.
	small := diagnoseApply(t, 256, 16, 4)
	large := diagnoseApply(t, 256, 256, 4)
	t.Logf("batch 16 -> %v total (%v/row); batch 256 -> %v total (%v/row)",
		small.total.Round(time.Millisecond), small.perRow.Round(time.Microsecond),
		large.total.Round(time.Millisecond), large.perRow.Round(time.Microsecond))
	fmt.Printf("APPLY_DIAG batch16_total=%d batch256_total=%d\n", small.total.Nanoseconds(), large.total.Nanoseconds())
}
