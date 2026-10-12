package runner

import (
	"context"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const maxPage = 128

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
		for i, row := range left.Rows {
			var change contracts.RowChange
			change.Expected = make(contracts.Row, len(table.Columns))
			change.Replacement = make(contracts.Row, len(table.Columns))
			for _, name := range table.PrimaryKey {
				l, a := row[name]
				r, b := right.Rows[i][name]
				if !a || !b || l.IsNull() || l != r {
					return visited, contracts.ErrConflict
				}
				change.Key = append(change.Key, l)
			}
			for _, col := range table.Columns {
				name := col.Column.Column
				old, a := row[name]
				current, b := right.Rows[i][name]
				if !a || !b || old != current {
					return visited, contracts.ErrConflict
				}
				newValue, err := e.maskColumn(ctx, old, col)
				if err != nil {
					return visited, err
				}
				change.Expected[name] = old
				change.Replacement[name] = newValue
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
	if !req.ExplicitApply || req.BatchSize < 1 || req.BatchSize > 1000 {
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
	writer, err := db.OpenWriter(ctx, opts)
	if err != nil {
		return report, err
	}
	defer writer.Close()

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
		changes := make([]contracts.RowChange, 0, len(page.Rows))
		for _, row := range page.Rows {
			change := contracts.RowChange{
				Expected: make(contracts.Row), Replacement: make(contracts.Row),
			}
			for _, pk := range table.PrimaryKey {
				v, ok := row[pk]
				if !ok || v.IsNull() {
					return report, contracts.ErrIncomplete
				}
				change.Key = append(change.Key, v)
			}
			for _, col := range table.Columns {
				v, ok := row[col.Column.Column]
				if !ok {
					return report, contracts.ErrIncomplete
				}
				out, err := e.maskColumn(ctx, v, col)
				if err != nil {
					return report, err
				}
				change.Expected[col.Column.Column] = v
				change.Replacement[col.Column.Column] = out
			}
			changes = append(changes, change)
		}
		tx, err := writer.Begin(ctx, table)
		if err != nil {
			return report, err
		}
		n, err := tx.Update(ctx, changes)
		if err != nil || n != int64(len(changes)) {
			_ = tx.Rollback(context.Background())
			if err == nil {
				err = contracts.ErrConflict
			}
			return report, err
		}
		if err = tx.Commit(ctx); err != nil {
			if err == contracts.ErrCommitUnknown {
				report.CommitOutcomeUnknown = true
			}
			return report, err
		}
		ordinal++
		report.Committed = append(report.Committed, contracts.BatchReceipt{
			Table: table.Table, Ordinal: ordinal, Rows: n,
		})
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
