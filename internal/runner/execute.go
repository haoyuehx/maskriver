package runner

import (
	"context"
	"errors"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const maxPage = 128

// requireSingleScope is the fail-closed WRITE capability gate. Preview and
// Validate read every planned table and column, but writing several tables or
// several columns in one run (work package B2) is not implemented yet, so Apply
// must refuse such a plan with ErrUnsupported instead of acting on a subset
// with partial coverage.
func requireSingleScope(plan contracts.Plan) error {
	if len(plan.Tables) == 1 && len(plan.Tables[0].Columns) == 1 {
		return nil
	}
	return contracts.ErrUnsupported
}

// planColumns is the minimum projection a table visit needs: the primary key
// followed by every planned column, in plan order.
func planColumns(table contracts.TablePlan) []string {
	cols := append([]string(nil), table.PrimaryKey...)
	for _, col := range table.Columns {
		cols = append(cols, col.Column.Column)
	}
	return cols
}

// visit walks every planned table in plan order and every planned column of
// each table, matching the two snapshots by the same stable primary key.
//
// budget is a PER-TABLE row budget: it restarts at every table, so an earlier
// table can never starve a later one. Coverage counts only fully visited
// tables, while Rows counts every row the visit actually examined, including
// rows of a table that was truncated. Complete is claimed only when every
// planned table was visited to its end, so a truncated, empty, drifted or
// cancelled visit always reports Complete=false.
//
// The callback never receives a secret through logs or a serialized report.
func (e *Engine) visit(ctx context.Context, plan contracts.Plan, budget int64, each func(contracts.RowChange) error) (contracts.Coverage, error) {
	var coverage contracts.Coverage
	if ctx == nil || budget < 0 {
		return coverage, contracts.ErrInvalid
	}
	var failure error
	for _, table := range plan.Tables {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		visited, err := e.visitTable(ctx, table, budget, each)
		coverage.Rows += visited
		switch {
		case err == nil:
			coverage.Tables++
			coverage.Columns += int64(len(table.Columns))
		case errors.Is(err, contracts.ErrIncomplete):
			// A truncated or empty table is not a completed table, but the
			// remaining tables are still visited so the report shows the real
			// extent instead of only the first shortfall.
			failure = contracts.ErrIncomplete
		default:
			// Cancellation, key drift and driver failures are terminal.
			return coverage, err
		}
	}
	coverage.Complete = failure == nil && coverage.Tables == int64(len(plan.Tables))
	return coverage, failure
}

// visitTable visits one planned table within its own row budget and emits one
// RowChange per visited row. It returns ErrIncomplete whenever the table was
// not visited to its end.
func (e *Engine) visitTable(ctx context.Context, table contracts.TablePlan, budget int64, each func(contracts.RowChange) error) (int64, error) {
	cols := planColumns(table)
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
	for {
		if err := ctx.Err(); err != nil {
			return visited, err
		}
		size := maxPage
		if budget > 0 && budget-visited < int64(size) {
			size = int(budget - visited)
		}
		if size <= 0 {
			// The budget ran out while rows remain: the visit is partial.
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
		changes, err := e.pageChanges(ctx, table, left.Rows, right.Rows)
		if err != nil {
			return visited, err
		}
		for _, change := range changes {
			if err := each(change); err != nil {
				return visited, err
			}
		}
		visited += int64(len(changes))
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

// pageChanges builds one RowChange per matched row of one page. Key holds the
// primary key values in plan order. Expected and Replacement only ever carry a
// planned column whose original value is not NULL, so a preview never claims a
// replacement it did not produce, and a row whose planned columns are all NULL
// still yields an (empty) RowChange. Every planned column is masked through its
// own ColumnPlan with the same bounded worker pool.
func (e *Engine) pageChanges(ctx context.Context, table contracts.TablePlan, left, right []contracts.Row) ([]contracts.RowChange, error) {
	changes := make([]contracts.RowChange, len(left))
	for i, row := range left {
		change := contracts.RowChange{Expected: contracts.Row{}, Replacement: contracts.Row{}}
		for _, name := range table.PrimaryKey {
			l, a := row[name]
			r, b := right[i][name]
			if !a || !b || l.IsNull() || l != r {
				return nil, contracts.ErrConflict
			}
			change.Key = append(change.Key, l)
		}
		changes[i] = change
	}
	for _, col := range table.Columns {
		name := col.Column.Column
		originals := make([]contracts.Value, 0, len(left))
		rows := make([]int, 0, len(left))
		for i := range left {
			old, a := left[i][name]
			current, b := right[i][name]
			if !a || !b || old != current {
				return nil, contracts.ErrConflict
			}
			if old.IsNull() {
				continue // NULL keeps no replacement
			}
			originals = append(originals, old)
			rows = append(rows, i)
		}
		if len(originals) == 0 {
			continue
		}
		masked, err := e.maskBatch(ctx, originals, col, e.cfg.Workers)
		if err != nil {
			return nil, err
		}
		for j, i := range rows {
			changes[i].Expected[name] = originals[j]
			changes[i].Replacement[name] = masked[j]
		}
	}
	return changes, nil
}

func (e *Engine) Preview(ctx context.Context, req contracts.PreviewRequest) (contracts.PreviewReport, error) {
	var report contracts.PreviewReport
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	// Preview reads every planned table and column, so the write-only
	// single-scope gate does not apply here. Structure is still validated
	// before anything else is read.
	if err := req.Plan.ValidateShape(); err != nil {
		return report, err
	}
	if err := e.checkPlan(ctx, req.Plan); err != nil {
		return report, err
	}
	// No Writer is opened, even for nonempty preview.
	coverage, err := e.visit(ctx, req.Plan, int64(req.LimitPerTable), func(contracts.RowChange) error { return nil })
	report.PlanID = req.Plan.ID
	report.RowsRead = coverage.Rows
	report.Coverage = coverage
	return report, err
}

func (e *Engine) Apply(ctx context.Context, req contracts.ApplyRequest) (contracts.ApplyReport, error) {
	var report contracts.ApplyReport
	report.PlanID = req.Plan.ID
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	if !req.ExplicitApply || !e.cfg.Approved || req.BatchSize < 1 || req.BatchSize > 1000 {
		return report, contracts.ErrUnsafe
	}
	if err := req.Plan.ValidateShape(); err != nil {
		return report, err
	}
	if err := requireSingleScope(req.Plan); err != nil {
		return report, err
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
