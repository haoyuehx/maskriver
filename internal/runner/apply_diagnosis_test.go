package runner

import (
	"context"
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
	rows       int64
	receipts   int
	batches    int
	statements int
	updates    int
	total      time.Duration
	perBatch   time.Duration
	perRow     time.Duration
}

// spyBatchWriter counts transactions and statements without changing what the
// writer does: every call is delegated to the real writer.
type spyBatchWriter struct {
	contracts.Writer
	mu       sync.Mutex
	begin    int
	commit   int
	rollback int
}

func (s *spyBatchWriter) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	s.mu.Lock()
	s.begin++
	s.mu.Unlock()
	tx, err := s.Writer.Begin(ctx, plan)
	if err != nil {
		return nil, err
	}
	return &spyTx{WriteTx: tx, parent: s}, nil
}

type spyTx struct {
	contracts.WriteTx
	parent *spyBatchWriter
}

func (t *spyTx) Commit(ctx context.Context) error {
	t.parent.mu.Lock()
	t.parent.commit++
	t.parent.mu.Unlock()
	return t.WriteTx.Commit(ctx)
}

func (t *spyTx) Rollback(ctx context.Context) error {
	t.parent.mu.Lock()
	t.parent.rollback++
	t.parent.mu.Unlock()
	return t.WriteTx.Rollback(ctx)
}

func diagnoseApply(t *testing.T, rows int, batchSize int, workers int) applyDiag {
	t.Helper()
	values := syntheticEmails(rows)
	fixture := newSignedEngine(t, values, workers)
	fixture.cfg.BatchSize = batchSize
	spy := db.NewSQLSpy()
	fixture.engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		writer, err := spy.OpenWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &spyBatchWriter{Writer: writer}, nil
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
	return applyDiag{
		rows: written, receipts: len(report.Committed), batches: len(report.Committed),
		statements: len(statements), updates: updates, total: total,
		perBatch: total / time.Duration(len(report.Committed)),
		perRow:   total / time.Duration(written),
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
		t.Logf("batch=%d rows=%d receipts=%d statements=%d updates=%d total=%v perBatch=%v perRow=%v",
			batchSize, diag.rows, diag.receipts, diag.statements, diag.updates,
			diag.total.Round(time.Microsecond), diag.perBatch.Round(time.Microsecond), diag.perRow.Round(time.Microsecond))
		if diag.rows != rows {
			t.Fatalf("batch=%d wrote %d rows, want %d", batchSize, diag.rows, rows)
		}
		if diag.receipts != (rows+batchSize-1)/batchSize {
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
