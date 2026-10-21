package runner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const maxPage = 128

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
func (e *Engine) visit(ctx context.Context, plan contracts.Plan, budget, maxRowBytes int64, each func(contracts.RowChange) error) (contracts.Coverage, error) {
	var coverage contracts.Coverage
	if ctx == nil || budget < 0 || maxRowBytes < 0 {
		return coverage, contracts.ErrInvalid
	}
	var failure error
	for _, table := range plan.Tables {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		visited, err := e.visitTable(ctx, table, budget, maxRowBytes, each)
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
func (e *Engine) visitTable(ctx context.Context, table contracts.TablePlan, budget, maxRowBytes int64, each func(contracts.RowChange) error) (int64, error) {
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
		left, err := pageWithin(ctx, e.source, request, maxRowBytes)
		if err != nil {
			return visited, err
		}
		right, err := pageWithin(ctx, e.target, request, maxRowBytes)
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
	// Preview is Apply's dry run, so the same whole-plan static capability gate
	// runs here: a plan that can not be applied is refused before any row is
	// read.
	if err := e.preflight(ctx, req.Plan); err != nil {
		return report, err
	}
	// No Writer is opened, even for nonempty preview.
	coverage, err := e.visit(ctx, req.Plan, int64(req.LimitPerTable), 0, func(contracts.RowChange) error { return nil })
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
//
// This gate proves STATIC conditions only, and only at the moment it runs. Schema
// and rows can still change between preflight and the first UPDATE, so the time
// of check to time of use window is not eliminated: the full dry run narrows it
// to the rows it just read, and the per-batch expected-value write keeps a
// changed row from being overwritten silently, but neither makes the source or
// target snapshot immutable for the length of the run.
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
// Both snapshots are enumerated, mirroring the trigger rule: the write lands in
// the target, so a referencing table - or a foreign key on a table present in
// both - that exists only on the target still guards the rows this plan changes.
// Enumerating the source alone missed that case and left the refusal to the
// driver, after a Writer had already begun a transaction.
//
// Names are compared case-insensitively because SQLite resolves identifiers that
// way: REFERENCES PEOPLE (EMAIL) points at table people, column email, while
// PRAGMA foreign_key_list reports the names exactly as the DDL spelled them. The
// fold is ASCII-only like SQLite's, and a wider match can only cause an extra
// refusal, never a missed one.
//
// A table whose foreign-key class was not fully described is ErrIncomplete: an
// unverified table can not be read as "references nothing". Masking a referenced
// column yields a fresh value that is not proven to still resolve, so the plan is
// refused with ErrUnsupported instead of guessed at. A reference is matched by
// target table name inside the described dataset; a name match across schemas can
// only cause an extra refusal, never a missed one.
func (e *Engine) checkIncomingReferences(ctx context.Context, plan contracts.Plan) error {
	modified := make(map[string]map[string]bool, len(plan.Tables))
	for _, table := range plan.Tables {
		columns := make(map[string]bool, len(table.Columns))
		for _, col := range table.Columns {
			columns[strings.ToLower(col.Column.Column)] = true
		}
		modified[strings.ToLower(table.Table.Table)] = columns
	}
	for _, side := range []contracts.Reader{e.source, e.target} {
		tables, err := side.Tables(ctx, nil)
		if err != nil {
			return err
		}
		for _, ref := range tables {
			if err := ctx.Err(); err != nil {
				return err
			}
			schema, err := side.Describe(ctx, ref)
			if err != nil {
				return err
			}
			if !schema.Coverage.ForeignKeys {
				return contracts.ErrIncomplete
			}
			for _, fk := range schema.ForeignKeys {
				columns, planned := modified[strings.ToLower(fk.Target.Table)]
				if !planned {
					continue
				}
				// An empty TargetColumns list means the parent's primary key, which a
				// plan can never modify (see ValidateShape's PK refusal).
				for _, name := range fk.TargetColumns {
					if columns[strings.ToLower(name)] {
						return contracts.ErrUnsupported
					}
				}
			}
		}
	}
	return nil
}

// dryRun proves the target still matches the source over every planned row and
// column. It reads only and never opens a Writer.
func (e *Engine) dryRun(ctx context.Context, plan contracts.Plan) error {
	_, err := e.visit(ctx, plan, 0, 0, func(contracts.RowChange) error { return nil })
	return err
}

// dryRunWithin is the streaming dry run. It applies the same row-byte budget as
// the apply loop, so a dry run can not be the one phase that materialises an
// oversized field. A reader without the bounded capability refuses the run.
func (e *Engine) dryRunWithin(ctx context.Context, plan contracts.Plan, maxRowBytes int64) error {
	_, err := e.visit(ctx, plan, 0, maxRowBytes, func(contracts.RowChange) error { return nil })
	return err
}

// pageWithin uses the internal byte-aware reader when a budget is declared and
// otherwise preserves the frozen Reader.Page semantics for legacy paths.
func pageWithin(ctx context.Context, reader contracts.Reader, req contracts.PageRequest, maxRowBytes int64) (contracts.Page, error) {
	if maxRowBytes <= 0 {
		return reader.Page(ctx, req)
	}
	bounded, ok := reader.(db.BoundedReader)
	if !ok {
		return contracts.Page{}, contracts.ErrUnsupported
	}
	return bounded.PageWithin(ctx, req, maxRowBytes)
}

// Apply writes every planned table and every planned column of the supplied
// plan. One Writer serves the whole run, one transaction covers each batch, and
// every batch reads BOTH snapshots with the same page request: a source-only
// read would let a concurrently changed target row be overwritten silently, so
// the target page is compared value-by-value against the source page through the
// shared visit seam before anything is written.
//
// The gates run in a fixed order and all of them pass before a Writer is opened:
// shape, explicit intent plus the operator's approval declaration, the
// authenticated review decision for exactly this plan, the whole-plan static
// preflight, and a full dry run over every planned table. A plan is therefore
// never executed on a subset of its tables, and a table this run can not prove
// writable refuses the whole plan.
//
// The review decision is an authentication gate, not detection evidence: it can
// not turn an UNKNOWN scan into an applicable plan, because the plan reaching
// Apply already carries complete Sensitive evidence or Plan refused to build it.
//
// Committed work is never hidden: the receipt of a batch is appended only after
// its Commit returned success, so a later failure keeps the earlier receipts in
// the report and leaves Complete false. A Commit whose outcome is unknown is
// neither retried nor treated as rolled back; execution stops and the report says
// CommitOutcomeUnknown.
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
	// A signed review decision bound to exactly this plan is required. It is
	// checked before any database work, so a missing, forged, foreign, expired,
	// revoked or unreadable decision refuses the run before a Writer exists.
	if err := e.checkApproval(ctx, req.Plan); err != nil {
		return report, err
	}
	if err := e.preflight(ctx, req.Plan); err != nil {
		return report, err
	}
	// Full dry run completes and proves the target matches source over every
	// planned row and column BEFORE opening a Writer. Unknown/partial scans
	// refuse to enter this path.
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
	run := &applyRun{engine: e, report: &report, batch: req.BatchSize}
	run.open = func(ctx context.Context) (contracts.Writer, error) { return openWriter(ctx, opts) }
	defer run.close()
	// Tables are visited in plan order, each with its own keyset cursor and its
	// own 1-based batch ordinal sequence. Any failure stops the run: a plan is
	// never partially reordered or resumed.
	for _, table := range req.Plan.Tables {
		if err := run.applyTable(ctx, table); err != nil {
			return report, err
		}
	}
	report.Complete = true
	return report, nil
}

// applyRun is the state of one Apply call. Every planned table shares the same
// Writer, which is opened on the first batch that has a row to write, so a run
// with nothing to write opens no Writer at all.
type applyRun struct {
	engine *Engine
	report *contracts.ApplyReport
	open   func(context.Context) (contracts.Writer, error)
	writer contracts.Writer
	batch  int
}

func (r *applyRun) close() {
	if r.writer != nil {
		_ = r.writer.Close()
	}
}

// writerFor returns the single Writer of the run, opening it on first use.
func (r *applyRun) writerFor(ctx context.Context) (contracts.Writer, error) {
	if r.writer != nil {
		return r.writer, nil
	}
	if r.open == nil {
		return nil, contracts.ErrInvalid
	}
	writer, err := r.open(ctx)
	if err != nil {
		return nil, err
	}
	r.writer = writer
	return writer, nil
}

// applyTable writes one planned table in batch-sized pages, each page in its own
// transaction. The cursor restarts at the first key for every table, and the
// batch ordinal restarts at 1, so a receipt names a position inside its own table.
//
// The table's row count on BOTH snapshots is recorded before its first page and
// compared again after its last page. The plan and its approval bind the planned
// scopes, not a row count, so a table that was non-empty at plan, preflight and
// dry-run time and is then emptied during the run would otherwise contribute
// zero rows and still be reported as Complete. Emptying both snapshots together
// is invisible to the page comparison, so the counts are the only check that
// notices: a count that differs between the snapshots or between start and end
// means the state the dry run proved no longer holds, and the run refuses with
// contracts.ErrConflict instead of claiming work it did not do.
func (r *applyRun) applyTable(ctx context.Context, table contracts.TablePlan) error {
	rows, err := r.countTable(ctx, table)
	if err != nil {
		return err
	}
	cols := planColumns(table)
	var after []contracts.Value
	var ordinal int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		request := contracts.PageRequest{
			Table: table.Table, Columns: cols, KeyColumns: table.PrimaryKey,
			After: after, Limit: r.batch,
		}
		left, err := r.engine.source.Page(ctx, request)
		if err != nil {
			return err
		}
		right, err := r.engine.target.Page(ctx, request)
		if err != nil {
			return err
		}
		// Both snapshots must return the same page: a diverging page, a diverging
		// end marker or a diverging cursor means the target no longer matches the
		// source, and continuing would mask rows whose current value is unknown.
		if len(left.Rows) != len(right.Rows) || left.Done != right.Done || len(left.Rows) > r.batch {
			return contracts.ErrConflict
		}
		if len(left.Rows) == 0 {
			// An exhausted table has nothing to write and needs no receipt.
			if left.Done {
				break
			}
			return contracts.ErrIncomplete
		}
		changes, err := r.engine.pageChanges(ctx, table, left.Rows, right.Rows)
		if err != nil {
			return err
		}
		committed, err := r.writeBatch(ctx, table, ordinal+1, changes)
		if err != nil {
			return err
		}
		if committed {
			ordinal++
		}
		if left.Done {
			break
		}
		if len(left.Next) != len(table.PrimaryKey) || len(right.Next) != len(table.PrimaryKey) {
			return contracts.ErrIncomplete
		}
		for i := range left.Next {
			if left.Next[i] != right.Next[i] {
				return contracts.ErrConflict
			}
		}
		after = append([]contracts.Value(nil), left.Next...)
	}
	end, err := r.countTable(ctx, table)
	if err != nil {
		return err
	}
	if end != rows {
		return contracts.ErrConflict
	}
	return nil
}

// countTable requires both snapshots to hold the same positive number of rows
// for one planned table and returns it. A missing table, a one-sided change or
// an emptied table is a conflict, not an empty success: the dry run that
// authorized this run proved the table non-empty, so zero rows means the state
// it proved is gone.
func (r *applyRun) countTable(ctx context.Context, table contracts.TablePlan) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	source, err := r.engine.source.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	target, err := r.engine.target.Count(ctx, table.Table)
	if err != nil {
		return 0, err
	}
	if source != target || source <= 0 {
		return 0, contracts.ErrConflict
	}
	return source, nil
}

// writeBatch writes one page in its own transaction. Rows whose planned columns
// are all NULL have nothing to write, so they are dropped before the transaction
// starts: such a row issues no statement and is never counted. The receipt is
// appended only after Commit returned success, which is what makes the report a
// record of durable work rather than of attempted work.
func (r *applyRun) writeBatch(ctx context.Context, table contracts.TablePlan, ordinal int64, changes []contracts.RowChange) (bool, error) {
	pending := make([]contracts.RowChange, 0, len(changes))
	for _, change := range changes {
		if assignsNothing(change) {
			continue
		}
		pending = append(pending, change)
	}
	if len(pending) == 0 {
		return false, nil
	}
	writer, err := r.writerFor(ctx)
	if err != nil {
		return false, err
	}
	tx, err := writer.Begin(ctx, table)
	if err != nil {
		return false, err
	}
	n, err := tx.Update(ctx, pending)
	if err != nil || n != int64(len(pending)) {
		_ = rollbackBounded(tx)
		if err == nil {
			err = contracts.ErrConflict
		}
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		if errors.Is(err, contracts.ErrCommitUnknown) {
			// The batch may or may not be durable: claim neither, retry never.
			r.report.CommitOutcomeUnknown = true
		} else {
			_ = rollbackBounded(tx)
		}
		return false, err
	}
	r.report.Committed = append(r.report.Committed, contracts.BatchReceipt{
		Table: table.Table, Ordinal: ordinal, Rows: n,
	})
	return true, nil
}

// assignsNothing reports whether every planned column of one change is NULL.
// Such a row needs no statement: the writer assigns only non-NULL columns, and a
// NULL original is never filled in.
func assignsNothing(change contracts.RowChange) bool {
	for _, value := range change.Expected {
		if !value.IsNull() {
			return false
		}
	}
	return true
}

func rollbackBounded(tx contracts.WriteTx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return tx.Rollback(ctx)
}
