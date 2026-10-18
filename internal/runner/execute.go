package runner

import (
	"context"
	"errors"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const maxPage = 128

// requireSingleScope is the fail-closed capability gate for preview, apply and
// validate. Multi-table and multi-column execution (work package B) is not
// implemented yet, so a plan that asks for more must be refused with
// ErrUnsupported instead of acting on a subset with partial coverage.
func requireSingleScope(plan contracts.Plan) error {
	if len(plan.Tables) == 1 && len(plan.Tables[0].Columns) == 1 {
		return nil
	}
	return contracts.ErrUnsupported
}

// visit walks two matching snapshots by the same stable primary key. The
// callback never receives a secret through logs or a serialized report.
func (e *Engine) visit(ctx context.Context, plan contracts.Plan, budget int64, each func(contracts.RowChange) error) (int64, error) {
	if ctx == nil || budget < 0 {
		return 0, contracts.ErrInvalid
	}
	table := plan.Tables[0]
	cols := append([]string(nil), table.PrimaryKey...)
	for _, col := range table.Columns {
		cols = append(cols, col.Column.Column)
	}
	var after []contracts.Value
	var visited int64
	srcCount, err := e.source.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	tgtCount, err := e.target.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	if srcCount != tgtCount || srcCount <= 0 {
		return 0, contracts.ErrIncomplete
	}
	if budget > 0 && srcCount > budget {
		return 0, contracts.ErrIncomplete
	}
	for {
		if err := ctx.Err(); err != nil {
			return visited, err
		}
		size := maxPage
		if budget > 0 && budget-visited < int64(size) {
			size = int(budget - visited)
		}
		if size <= 0 {
			return visited, contracts.ErrIncomplete
		}
		request := contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: size,
		}
		left, err := e.source.Page(ctx, request)
		if err != nil {
			return visited, err
		}
		right, err := e.target.Page(ctx, request)
		if err != nil {
			return visited, err
		}
		if len(left.Rows) == 0 || len(left.Rows) != len(right.Rows) ||
			left.Done != right.Done || len(left.Rows) > size {
			return visited, contracts.ErrIncomplete
		}
		if len(table.Columns) != 1 {
			return visited, contracts.ErrUnsupported
		}
		col := table.Columns[0]
		originals := make([]contracts.Value, len(left.Rows))
		for i, row := range left.Rows {
			for _, name := range table.PrimaryKey {
				l, a := row[name]
				r, b := right.Rows[i][name]
				if !a || !b || l.IsNull() || l != r {
					return visited, contracts.ErrConflict
				}
			}
			old, a := row[col.Column.Column]
			current, b := right.Rows[i][col.Column.Column]
			if !a || !b || old != current {
				return visited, contracts.ErrConflict
			}
			originals[i] = old
		}
		masked, err := e.maskBatch(ctx, originals, col, e.cfg.Workers)
		if err != nil {
			return visited, err
		}
		for i, row := range left.Rows {
			change := contracts.RowChange{
				Expected:    contracts.Row{col.Column.Column: originals[i]},
				Replacement: contracts.Row{col.Column.Column: masked[i]},
			}
			for _, name := range table.PrimaryKey {
				change.Key = append(change.Key, row[name])
			}
			if err := each(change); err != nil {
				return visited, err
			}
			visited++
		}
		if left.Done {
			break
		}
		if len(left.Next) != len(table.PrimaryKey) ||
			len(right.Next) != len(table.PrimaryKey) {
			return visited, contracts.ErrIncomplete
		}
		for i := range left.Next {
			if left.Next[i] != right.Next[i] {
				return visited, contracts.ErrConflict
			}
		}
		after = append([]contracts.Value(nil), left.Next...)
	}
	if visited != srcCount {
		return visited, contracts.ErrIncomplete
	}
	return visited, nil
}

func (e *Engine) Preview(ctx context.Context, req contracts.PreviewRequest) (contracts.PreviewReport, error) {
	var report contracts.PreviewReport
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	// Multi-table and multi-column execution is not implemented yet, so those
	// plans must fail closed before any row is read.
	if err := requireSingleScope(req.Plan); err != nil {
		return report, err
	}
	if err := e.checkPlan(ctx, req.Plan); err != nil {
		return report, err
	}
	// No Writer is opened, even for nonempty preview.
	n, err := e.visit(ctx, req.Plan, int64(req.LimitPerTable), func(contracts.RowChange) error { return nil })
	report.PlanID = req.Plan.ID
	report.RowsRead = n
	report.Coverage = contracts.Coverage{
		Tables: 1, Columns: int64(len(req.Plan.Tables[0].Columns)),
		Rows: n, Complete: err == nil,
	}
	return report, err
}

func (e *Engine) Apply(ctx context.Context, req contracts.ApplyRequest) (contracts.ApplyReport, error) {
	var report contracts.ApplyReport
	report.PlanID = req.Plan.ID
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	if err := requireSingleScope(req.Plan); err != nil {
		return report, err
	}
	if !req.ExplicitApply || !e.cfg.Approved || req.BatchSize < 1 || req.BatchSize > 1000 {
		return report, contracts.ErrUnsafe
	}
	if err := e.checkPlan(ctx, req.Plan); err != nil {
		return report, err
	}
	// Full dry run completes and proves the target matches source BEFORE
	// opening a Writer. Unknown/partial scans refuse to enter this path.
	if _, err := e.Preview(ctx, contracts.PreviewRequest{Plan: req.Plan}); err != nil {
		return report, err
	}
	opts, err := e.cfg.DatabaseOptions(false)
	if err != nil {
		return report, err
	}
	openWriter := e.openWriter
	if openWriter == nil {
		openWriter = db.OpenWriter
	}
	var writer contracts.Writer

	table := req.Plan.Tables[0]
	cols := append([]string(nil), table.PrimaryKey...)
	for _, c := range table.Columns {
		cols = append(cols, c.Column.Column)
	}
	var after []contracts.Value
	var ordinal int64
	for {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		page, err := e.source.Page(ctx, contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: req.BatchSize,
		})
		if err != nil {
			return report, err
		}
		if len(page.Rows) == 0 {
			return report, contracts.ErrIncomplete
		}
		if len(table.Columns) != 1 {
			return report, contracts.ErrUnsupported
		}
		col := table.Columns[0]
		originals := make([]contracts.Value, len(page.Rows))
		for i, row := range page.Rows {
			v, ok := row[col.Column.Column]
			if !ok {
				return report, contracts.ErrIncomplete
			}
			originals[i] = v
		}
		masked, err := e.maskBatch(ctx, originals, col, e.cfg.Workers)
		if err != nil {
			return report, err
		}
		changes := make([]contracts.RowChange, 0, len(page.Rows))
		for i, row := range page.Rows {
			change := contracts.RowChange{
				Expected:    contracts.Row{col.Column.Column: originals[i]},
				Replacement: contracts.Row{col.Column.Column: masked[i]},
			}
			for _, pk := range table.PrimaryKey {
				v, ok := row[pk]
				if !ok || v.IsNull() {
					return report, contracts.ErrIncomplete
				}
				change.Key = append(change.Key, v)
			}
			if !originals[i].IsNull() {
				changes = append(changes, change)
			}
		}
		if len(changes) > 0 {
			if writer == nil {
				writer, err = openWriter(ctx, opts)
				if err != nil {
					return report, err
				}
				defer writer.Close()
			}
			tx, err := writer.Begin(ctx, table)
			if err != nil {
				return report, err
			}
			n, err := tx.Update(ctx, changes)
			if err != nil || n != int64(len(changes)) {
				_ = rollbackBounded(tx)
				if err == nil {
					err = contracts.ErrConflict
				}
				return report, err
			}
			if err = tx.Commit(ctx); err != nil {
				if errors.Is(err, contracts.ErrCommitUnknown) {
					report.CommitOutcomeUnknown = true
				} else {
					_ = rollbackBounded(tx)
				}
				return report, err
			}
			ordinal++
			report.Committed = append(report.Committed, contracts.BatchReceipt{
				Table: table.Table, Ordinal: ordinal, Rows: n,
			})
		}
		if page.Done {
			report.Complete = true
			return report, nil
		}
		if len(page.Next) != len(table.PrimaryKey) {
			return report, contracts.ErrIncomplete
		}
		after = append([]contracts.Value(nil), page.Next...)
	}
}

func rollbackBounded(tx contracts.WriteTx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return tx.Rollback(ctx)
}
