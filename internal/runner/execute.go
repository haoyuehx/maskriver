package runner

import (
	"context"
	"errors"
	"reflect"
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
// table can never starve a later one; 0 means no row limit. The page size is
// capped at maxPage independently of the budget.
//
// Coverage counts only fully visited tables, while Rows counts every row the
// visit actually examined, including rows of a table that was truncated. Complete is claimed only when every
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
// primary key values in plan order. Expected and Replacement carry every
// planned column, so the writer sees the same column set the plan declares; a
// NULL original is carried as NULL in both rows, because nothing may replace it
// and the column is therefore never assigned. Every non-NULL original is masked
// through its own ColumnPlan with the same bounded worker pool Apply uses.
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
				// A NULL original stays NULL: it is carried in both rows so the
				// column set matches the plan, and it is never masked or assigned.
				changes[i].Expected[name] = old
				changes[i].Replacement[name] = old
				continue
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
	// Preview is Apply's dry run, so the same static capability gate runs here:
	// a plan that can not be applied is refused before any row is read. The
	// write-only single-scope gate still does not apply to Preview.
	if err := e.preflight(ctx, req.Plan); err != nil {
		return report, err
	}
	// No Writer is opened, even for nonempty preview.
	coverage, err := e.visit(ctx, req.Plan, int64(req.LimitPerTable), func(contracts.RowChange) error { return nil })
	report.PlanID = req.Plan.ID
	report.RowsRead = coverage.Rows
	report.Coverage = coverage
	return report, err
}

// preflight is the whole-plan static capability gate. Apply runs it before it
// creates any Writer, and Preview runs the same gate so a plan that could never
// be applied is refused before any row is read.
//
// Every fact the gate needs is re-derived from the live database and the
// declared scopes instead of trusted from the supplied plan: shape, the
// recomputed plan identity, the live schemas with complete coverage, and the
// static DDL capabilities of every planned table. A table or column combination
// this gate can not prove writable refuses the whole plan; a partial write is
// never authorized.
func (e *Engine) preflight(ctx context.Context, plan contracts.Plan) error {
	if err := plan.ValidateShape(); err != nil {
		return err
	}
	if err := e.checkPlan(ctx, plan); err != nil {
		return err
	}
	schemas, err := e.describePlanTables(ctx, plan)
	if err != nil {
		return err
	}
	if err := e.checkPlanBinding(plan, schemas); err != nil {
		return err
	}
	for _, table := range plan.Tables {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.checkTableCapabilities(ctx, table, schemas[table.Table.Table]); err != nil {
			return err
		}
	}
	return e.checkIncomingReferences(ctx, plan)
}

// describePlanTables requires a complete live description of every planned
// table on both snapshots. describe already proves source==target for those
// tables and a stable primary key; the coverage check adds the constraint
// classes the capability rules read, because a class that was not described
// must not be read as absent.
func (e *Engine) describePlanTables(ctx context.Context, plan contracts.Plan) (map[string]contracts.TableSchema, error) {
	schemas := make(map[string]contracts.TableSchema, len(plan.Tables))
	for _, table := range plan.Tables {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		schema, err := e.describe(ctx, table.Table.Table)
		if err != nil {
			return nil, err
		}
		if !completeCoverage(schema.Coverage) {
			return nil, contracts.ErrIncomplete
		}
		schemas[table.Table.Table] = schema
	}
	return schemas, nil
}

// completeCoverage reports whether Describe verified every schema class the
// static capability rules depend on.
func completeCoverage(c contracts.SchemaCoverage) bool {
	return c.Columns && c.PrimaryKey && c.Indexes && c.Unique && c.ForeignKeys && c.Checks
}

// checkPlanBinding re-verifies the plan's own binding against the declared
// scopes and the live schema. ValidateShape runs first, but the plan is caller
// supplied, so nothing is assumed: every planned column must belong to its own
// table, the declared scope list must be covered exactly once in declared
// order, and every planned column must match the live declared type.
func (e *Engine) checkPlanBinding(plan contracts.Plan, schemas map[string]contracts.TableSchema) error {
	// Plan groups scopes by first appearance of their table and keeps the
	// declared column order inside each table, so the same grouping is rebuilt
	// here from the declared configuration.
	type expectedTable struct {
		name    string
		columns []string
	}
	var expected []expectedTable
	index := make(map[string]int, len(plan.Tables))
	for _, scope := range e.cfg.ResolvedScopes() {
		i, seen := index[scope.Table]
		if !seen {
			i = len(expected)
			index[scope.Table] = i
			expected = append(expected, expectedTable{name: scope.Table})
		}
		expected[i].columns = append(expected[i].columns, scope.Column)
	}
	if len(plan.Tables) != len(expected) {
		return contracts.ErrInvalid
	}
	for i, table := range plan.Tables {
		if table.Table.Table != expected[i].name || len(table.Columns) != len(expected[i].columns) {
			return contracts.ErrInvalid
		}
		schema, ok := schemas[table.Table.Table]
		if !ok || !reflect.DeepEqual(table.PrimaryKey, schema.PrimaryKey) {
			return contracts.ErrInvalid
		}
		for j, col := range table.Columns {
			if col.Column.TableRef != table.Table || col.Column.Column != expected[i].columns[j] {
				return contracts.ErrInvalid
			}
			live, ok := columnByName(schema, col.Column.Column)
			if !ok || live.Type != col.Type {
				return contracts.ErrInvalid
			}
		}
	}
	return nil
}

// checkTableCapabilities applies the static DDL rules for one planned table.
//
// UNIQUE: masking is deterministic but not proven collision-free, so a modified
// column inside a unique index or constraint can collide and break it. Refuse
// with ErrUnsafe.
// CHECK: the masked value may violate a CHECK expression that mentions the
// column, and this gate does not evaluate SQL expressions. Refuse with
// ErrUnsupported.
// FOREIGN KEY: a modified column that is itself a foreign-key column points at
// another table's key, and the replacement is a fresh value. Refuse with
// ErrUnsupported.
// TRIGGER: a trigger body is arbitrary SQL, so any trigger on a table this plan
// writes is refused; SQLite DDL does not attribute a trigger to a column, so an
// "unrelated" trigger is not provable. See db.HasTriggers.
// A plain non-unique index carries no constraint: changing an indexed column
// only makes SQLite maintain that index, and no value can violate it, so plain
// indexes are allowed rather than refused.
func (e *Engine) checkTableCapabilities(ctx context.Context, table contracts.TablePlan, schema contracts.TableSchema) error {
	modified := make(map[string]bool, len(table.Columns))
	for _, col := range table.Columns {
		modified[col.Column.Column] = true
	}
	// Triggers must be absent on BOTH snapshots: the write lands in the target,
	// so a trigger that exists only there still fires silently. Checking the
	// source alone was a fail-open gap.
	for _, side := range []contracts.Reader{e.source, e.target} {
		caps, err := sourceCapabilities(side)
		if err != nil {
			return err
		}
		has, err := caps.HasTriggers(ctx, table.Table)
		if err != nil {
			return err
		}
		if has {
			return contracts.ErrUnsupported
		}
	}
	for _, unique := range schema.Unique {
		for _, name := range unique {
			if modified[name] {
				return contracts.ErrUnsafe
			}
		}
	}
	for _, index := range schema.Indexes {
		if !index.Unique {
			continue
		}
		for _, name := range index.Columns {
			if modified[name] {
				return contracts.ErrUnsafe
			}
		}
	}
	for _, check := range schema.Checks {
		for name := range modified {
			if db.CheckMentionsColumn(check, name) {
				return contracts.ErrUnsupported
			}
		}
	}
	for _, fk := range schema.ForeignKeys {
		for _, name := range fk.Columns {
			if modified[name] {
				return contracts.ErrUnsupported
			}
		}
	}
	return nil
}

// checkIncomingReferences refuses a plan that modifies a column another table's
// foreign key points at. The reference lives in the referencing table, not in
// the planned table's schema, so every table of the dataset is described.
//
// A table whose foreign-key class was not fully described is ErrIncomplete: an
// unverified table can not be read as "references nothing". Masking a
// referenced column yields a fresh value that is not proven to still resolve,
// so the plan is refused with ErrUnsupported instead of guessed at. A reference
// is matched by target table name inside the described dataset; a name match
// across schemas can only cause an extra refusal, never a missed one.
func (e *Engine) checkIncomingReferences(ctx context.Context, plan contracts.Plan) error {
	modified := make(map[string]map[string]bool, len(plan.Tables))
	for _, table := range plan.Tables {
		columns := make(map[string]bool, len(table.Columns))
		for _, col := range table.Columns {
			columns[col.Column.Column] = true
		}
		modified[table.Table.Table] = columns
	}
	tables, err := e.source.Tables(ctx, nil)
	if err != nil {
		return err
	}
	for _, ref := range tables {
		if err := ctx.Err(); err != nil {
			return err
		}
		schema, err := e.source.Describe(ctx, ref)
		if err != nil {
			return err
		}
		if !schema.Coverage.ForeignKeys {
			return contracts.ErrIncomplete
		}
		for _, fk := range schema.ForeignKeys {
			columns, planned := modified[fk.Target.Table]
			if !planned {
				continue
			}
			// An empty TargetColumns list means the parent's primary key, which a
			// plan can never modify (see ValidateShape's PK refusal).
			for _, name := range fk.TargetColumns {
				if columns[name] {
					return contracts.ErrUnsupported
				}
			}
		}
	}
	return nil
}

// dryRun proves the target still matches the source over every planned row and
// column. It reads only and never opens a Writer.
func (e *Engine) dryRun(ctx context.Context, plan contracts.Plan) error {
	_, err := e.visit(ctx, plan, 0, func(contracts.RowChange) error { return nil })
	return err
}

func (e *Engine) Apply(ctx context.Context, req contracts.ApplyRequest) (contracts.ApplyReport, error) {
	var report contracts.ApplyReport
	report.PlanID = req.Plan.ID
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	// Structure is validated before authorization, the same order Preview and
	// Validate use, so a malformed plan is always reported as malformed.
	if err := req.Plan.ValidateShape(); err != nil {
		return report, err
	}
	if !req.ExplicitApply || !e.cfg.Approved || req.BatchSize < 1 || req.BatchSize > 1000 {
		return report, contracts.ErrUnsafe
	}
	// The write-capability gate runs before the more expensive static preflight:
	// a plan Apply can not execute at all is refused first.
	if err := requireSingleScope(req.Plan); err != nil {
		return report, err
	}
	if err := e.preflight(ctx, req.Plan); err != nil {
		return report, err
	}
	// Full dry run completes and proves the target matches source BEFORE
	// opening a Writer. Unknown/partial scans refuse to enter this path.
	if err := e.dryRun(ctx, req.Plan); err != nil {
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
		name := col.Column.Column
		originals := make([]contracts.Value, 0, len(page.Rows))
		indexes := make([]int, 0, len(page.Rows))
		for i, row := range page.Rows {
			v, ok := row[name]
			if !ok {
				return report, contracts.ErrIncomplete
			}
			if v.IsNull() {
				// A NULL original stays NULL, so it is neither masked nor written
				// and the row is excluded from the batch.
				continue
			}
			originals = append(originals, v)
			indexes = append(indexes, i)
		}
		masked, err := e.maskBatch(ctx, originals, col, e.cfg.Workers)
		if err != nil {
			return report, err
		}
		changes := make([]contracts.RowChange, 0, len(indexes))
		for j, i := range indexes {
			change := contracts.RowChange{
				Expected:    contracts.Row{name: originals[j]},
				Replacement: contracts.Row{name: masked[j]},
			}
			for _, pk := range table.PrimaryKey {
				v, ok := page.Rows[i][pk]
				if !ok || v.IsNull() {
					return report, contracts.ErrIncomplete
				}
				change.Key = append(change.Key, v)
			}
			changes = append(changes, change)
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
