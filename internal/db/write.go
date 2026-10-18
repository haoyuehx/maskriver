package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

type writeTx struct {
	owner    *writer
	tx       *sql.Tx
	plan     contracts.TablePlan
	keyTypes []contracts.Type
	changed  map[string]contracts.Type
	ordered  []string
	mu       sync.Mutex
	ended    bool
}

var _ contracts.WriteTx = (*writeTx)(nil)

func (w *writer) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	schema, err := w.describe(ctx, plan.Table)
	if err != nil {
		return nil, err
	}
	if len(schema.PrimaryKey) == 0 {
		return nil, contracts.ErrUnsupported
	}
	if len(plan.PrimaryKey) != len(schema.PrimaryKey) || len(plan.Columns) == 0 {
		return nil, contracts.ErrInvalid
	}
	byName := map[string]contracts.Type{}
	for _, c := range schema.Columns {
		byName[c.Ref.Column] = c.Type
	}
	keyTypes := make([]contracts.Type, len(plan.PrimaryKey))
	for i, key := range plan.PrimaryKey {
		if key != schema.PrimaryKey[i] {
			return nil, contracts.ErrInvalid
		}
		typ, ok := byName[key]
		if !ok || typ.Nullable {
			return nil, contracts.ErrUnsupported
		}
		keyTypes[i] = typ
	}
	changed := map[string]contracts.Type{}
	ordered := make([]string, 0, len(plan.Columns))
	for _, col := range plan.Columns {
		if col.Column.TableRef != plan.Table || col.Column.Column == "" {
			return nil, contracts.ErrInvalid
		}
		name := col.Column.Column
		typ, ok := byName[name]
		if !ok || col.Type != typ {
			return nil, contracts.ErrInvalid
		}
		for _, key := range plan.PrimaryKey {
			if name == key {
				return nil, contracts.ErrUnsafe
			}
		}
		if _, exists := changed[name]; exists {
			return nil, contracts.ErrInvalid
		}
		changed[name] = typ
		ordered = append(ordered, name)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, contracts.ErrClosed
	}
	if w.active != nil {
		return nil, contracts.ErrUnsafe
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, dbError(err)
	}
	result := &writeTx{owner: w, tx: tx, plan: plan, keyTypes: keyTypes, changed: changed, ordered: ordered}
	w.active = result
	return result, nil
}

// Update applies one change per row. A change whose planned columns are all
// NULL assigns nothing (the original stays NULL), so it is skipped without
// issuing a statement and is not counted; every other change is exactly one
// parameterised UPDATE inside the caller's transaction. The returned count is
// the number of rows actually updated, so a caller that wants to compare it
// with its own list length must exclude the all-NULL rows first.
func (t *writeTx) Update(ctx context.Context, changes []contracts.RowChange) (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return 0, contracts.ErrClosed
	}
	if ctx == nil || len(changes) == 0 {
		t.abortLocked()
		return 0, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		t.abortLocked()
		return 0, err
	}
	// Callers must never ask for an empty batch: the frozen Writer contract
	// requires every item in a batch to affect exactly one row, so an empty
	// call is a caller bug rather than a successful no-op.
	if len(changes) == 0 {
		t.abortLocked()
		return 0, contracts.ErrInvalid
	}
	var total int64
	for _, change := range changes {
		if err := t.validateChange(change); err != nil {
			t.abortLocked()
			return 0, err
		}
		assigned := t.assignedColumns(change)
		if len(assigned) == 0 {
			continue
		}
		query, args := t.updateSQL(change, assigned)
		result, err := t.tx.ExecContext(ctx, query, args...)
		if err != nil {
			t.abortLocked()
			return 0, dbError(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			t.abortLocked()
			return 0, contracts.ErrDatabase
		}
		if affected != 1 {
			t.abortLocked()
			return 0, contracts.ErrConflict
		}
		total++
	}
	return total, nil
}

// validateChange enforces the complete change contract for one row: the key
// types match, Expected and Replacement carry every planned column of the plan,
// and the only legal move for a NULL original is to stay NULL. A non-NULL
// original whose replacement equals it is refused, so no change can claim a
// write it would not perform.
func (t *writeTx) validateChange(change contracts.RowChange) error {
	if len(change.Key) != len(t.keyTypes) ||
		len(change.Expected) != len(t.changed) ||
		len(change.Replacement) != len(t.changed) {
		return contracts.ErrInvalid
	}
	for i, v := range change.Key {
		if v.IsNull() || v.Kind() != t.keyTypes[i].Kind {
			return contracts.ErrInvalid
		}
	}
	for _, name := range t.ordered {
		typ := t.changed[name]
		expected, ok := change.Expected[name]
		if !ok {
			return contracts.ErrInvalid
		}
		replacement, ok := change.Replacement[name]
		if !ok {
			return contracts.ErrInvalid
		}
		if err := safeType(expected, typ); err != nil {
			return err
		}
		if err := safeType(replacement, typ); err != nil {
			return err
		}
		if expected.IsNull() {
			if !replacement.IsNull() {
				// A NULL original may only stay NULL: filling it in would write a
				// value this run never read.
				return contracts.ErrUnsafe
			}
			continue
		}
		if expected == replacement {
			return contracts.ErrUnsafe
		}
	}
	return nil
}

// assignedColumns reports the planned columns one UPDATE must assign, in plan
// order. A NULL expected value is never assigned, because the original stays
// NULL; a non-NULL column is assigned only when its replacement differs from it.
// An empty result means the row has nothing to write.
func (t *writeTx) assignedColumns(change contracts.RowChange) []string {
	names := make([]string, 0, len(t.ordered))
	for _, name := range t.ordered {
		if change.Expected[name].IsNull() || change.Expected[name] == change.Replacement[name] {
			continue
		}
		names = append(names, name)
	}
	return names
}

// updateSQL builds the single parameterised UPDATE of one row: the SET clause
// carries only the assigned columns, and the WHERE clause guards the primary key
// plus every planned column, whether or not it is assigned, so a concurrent
// change to any planned column is still a conflict.
func (t *writeTx) updateSQL(change contracts.RowChange, assigned []string) (string, []any) {
	a := t.owner.adapter
	assignments := make([]string, 0, len(assigned))
	predicates := make([]string, 0, len(t.plan.PrimaryKey)+len(t.ordered))
	args := make([]any, 0, len(assigned)+len(t.plan.PrimaryKey)+len(t.ordered))
	for _, name := range assigned {
		assignments = append(assignments, a.quote(name)+"=?")
		args = append(args, encode(change.Replacement[name]))
	}
	for i, name := range t.plan.PrimaryKey {
		predicates = append(predicates, a.quote(name)+"=?")
		args = append(args, encode(change.Key[i]))
	}
	for _, name := range t.ordered {
		op := " IS ?"
		if a.dialect == contracts.MySQL {
			op = " <=> ?"
		}
		predicates = append(predicates, a.quote(name)+op)
		args = append(args, encode(change.Expected[name]))
	}
	query := "UPDATE " + a.qualified(t.plan.Table) + " SET " + strings.Join(assignments, ",") + " WHERE " + strings.Join(predicates, " AND ")
	return query, args
}

func (t *writeTx) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return contracts.ErrClosed
	}
	if ctx == nil {
		t.abortLocked()
		return contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		t.abortLocked()
		return err
	}
	err := t.tx.Commit()
	t.endLocked()
	if err != nil {
		// After a commit attempt the outcome may be unknown; never auto-retry.
		return contracts.ErrCommitUnknown
	}
	return nil
}

func (t *writeTx) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return nil
	}
	if ctx == nil {
		return contracts.ErrInvalid
	}
	err := t.tx.Rollback()
	t.endLocked()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return contracts.ErrDatabase
	}
	return nil
}

func (t *writeTx) abortLocked() {
	_ = t.tx.Rollback()
	t.endLocked()
}

func (t *writeTx) endLocked() {
	t.ended = true
	w := t.owner
	w.mu.Lock()
	if w.active == t {
		w.active = nil
	}
	w.mu.Unlock()
}
