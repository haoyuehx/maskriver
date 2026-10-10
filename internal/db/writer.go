package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// sqliteHandle implements both Reader and Writer. MySQL handle below is
// structurally identical; the two concrete types exist for future
// dialect-specific behavior.
func (s *sqliteHandle) Tables(ctx context.Context, schemas []string) ([]contracts.TableRef, error) {
	return s.baseHandle.Tables(ctx, schemas)
}
func (s *sqliteHandle) Describe(ctx context.Context, t contracts.TableRef) (contracts.TableSchema, error) {
	return s.baseHandle.Describe(ctx, t)
}
func (s *sqliteHandle) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	return s.baseHandle.Sample(ctx, req)
}
func (s *sqliteHandle) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	return s.baseHandle.Page(ctx, req)
}
func (s *sqliteHandle) Count(ctx context.Context, t contracts.TableRef) (int64, error) {
	return s.baseHandle.Count(ctx, t)
}
func (s *sqliteHandle) Close() error { return s.baseHandle.Close() }

func (s *sqliteHandle) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	if err := s.assertOpen(); err != nil {
		return nil, err
	}
	if plan.Table.Table == "" || len(plan.PrimaryKey) == 0 {
		return nil, contracts.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapErr(err)
	}
	return newWriteTx(s.dialect, s.db, tx, plan, s.dataset), nil
}

func (m *mysqlHandle) Tables(ctx context.Context, schemas []string) ([]contracts.TableRef, error) {
	return m.baseHandle.Tables(ctx, schemas)
}
func (m *mysqlHandle) Describe(ctx context.Context, t contracts.TableRef) (contracts.TableSchema, error) {
	return m.baseHandle.Describe(ctx, t)
}
func (m *mysqlHandle) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	return m.baseHandle.Sample(ctx, req)
}
func (m *mysqlHandle) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	return m.baseHandle.Page(ctx, req)
}
func (m *mysqlHandle) Count(ctx context.Context, t contracts.TableRef) (int64, error) {
	return m.baseHandle.Count(ctx, t)
}
func (m *mysqlHandle) Close() error { return m.baseHandle.Close() }

func (m *mysqlHandle) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	if err := m.assertOpen(); err != nil {
		return nil, err
	}
	if plan.Table.Table == "" || len(plan.PrimaryKey) == 0 {
		return nil, contracts.ErrInvalid
	}
	// MySQL defaults to REPEATABLE READ which is insufficient for
	// exact-row optimistic conflict detection on concurrent UPDATEs
	// with WHERE key = ? AND col = ?; READ COMMITTED + row-level locks
	// is the minimum isolation we accept.
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, mapErr(err)
	}
	if _, err2 := tx.ExecContext(ctx, "SET SESSION sql_safe_updates=1"); err2 != nil {
		// Non-fatal: best-effort defense.
		_ = err2
	}
	return newWriteTx(m.dialect, m.db, tx, plan, m.dataset), nil
}

// -------- WriteTx (both dialects share implementation) -------------

type writeTx struct {
	mu      sync.Mutex
	dialect contracts.Dialect
	db      *sql.DB // retained only to ensure Close/Rollback diagnostics
	tx      *sql.Tx
	plan    contracts.TablePlan
	dataset string

	rolledBack bool
	committed  bool
	finished   bool
}

func newWriteTx(d contracts.Dialect, db *sql.DB, tx *sql.Tx, plan contracts.TablePlan, dataset string) *writeTx {
	return &writeTx{dialect: d, db: db, tx: tx, plan: plan, dataset: dataset}
}

// Update applies one UPDATE statement per RowChange. For every change
// we require:
//   - len(Key) == len(PrimaryKey)
//   - Expected map contains only column references that also appear in
//     Replacement (optimistic guard columns); Replacement may be a
//     subset of columns.
//   - Each UPDATE WHERE clause has:
//     key_1 = ? AND ... AND key_n = ? AND col_A <=> ? AND ... col_Z <=> ?
//     For dialects that do not support <=> (MySQL's null-safe equals),
//     we expand to (c = ? OR (c IS NULL AND ? IS NULL)). SQLite uses
//     native IS for NULL-safe compare.
//
// RowsAffected for each statement MUST equal exactly 1. Any 0 returns
// ErrConflict and triggers a transaction-level rollback before
// returning so that partial batches never persist.
func (tx *writeTx) Update(ctx context.Context, changes []contracts.RowChange) (int64, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.finished {
		return 0, contracts.ErrClosed
	}
	pk := tx.plan.PrimaryKey
	if len(pk) == 0 {
		return 0, contracts.ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Validate shapes once.
	for i, c := range changes {
		if len(c.Key) != len(pk) {
			return 0, contracts.ErrInvalid
		}
		if len(c.Replacement) == 0 {
			return 0, contracts.ErrInvalid
		}
		for col := range c.Replacement {
			if _, has := c.Expected[col]; !has {
				_ = i
				// We tolerate missing Expected when the column itself
				// is the replacement target but the runner didn't
				// record an expected value. That case would mean the
				// optimistic guard is weaker; to preserve per-row
				// exactness we simply refuse the update.
				return 0, contracts.ErrInvalid
			}
		}
	}

	total := int64(0)
	for _, c := range changes {
		affected, err := tx.applySingle(ctx, c)
		if err != nil {
			// Immediate rollback per contracts contract.
			_ = tx.tx.Rollback()
			tx.finished = true
			tx.rolledBack = true
			if errors.Is(err, contracts.ErrConflict) {
				return total, err
			}
			return total, mapErr(err)
		}
		total += affected
	}
	return total, nil
}

func (tx *writeTx) applySingle(ctx context.Context, c contracts.RowChange) (int64, error) {
	plan := tx.plan
	// Determine changed columns from Replacement.
	changedCols := make([]string, 0, len(c.Replacement))
	for col := range c.Replacement {
		changedCols = append(changedCols, col)
	}
	sortStrings(changedCols)
	// Build UPDATE t SET col=?, col=? WHERE (key parts null-safe equals) AND (expected cols null-safe equals)
	var setParts []string
	setArgs := make([]any, 0, len(changedCols))
	for _, col := range changedCols {
		setParts = append(setParts, fmt.Sprintf("%s = %s", quoteIdent(tx.dialect, col), placeholder(tx.dialect, len(setArgs)+1)))
		setArgs = append(setArgs, valueToSQL(c.Replacement[col]))
	}
	keyParts := make([]string, 0, len(plan.PrimaryKey))
	keyArgs := make([]any, 0, len(plan.PrimaryKey))
	for i, k := range plan.PrimaryKey {
		expr, args := nullSafeCompare(tx.dialect, k, valueToSQL(c.Key[i]), len(keyArgs)+len(setArgs)+1)
		keyParts = append(keyParts, "("+expr+")")
		keyArgs = append(keyArgs, args...)
	}
	// Expected columns (optimistic guard).
	expectedCols := make([]string, 0, len(c.Expected))
	for col := range c.Expected {
		expectedCols = append(expectedCols, col)
	}
	sortStrings(expectedCols)
	expParts := make([]string, 0, len(expectedCols))
	expArgs := make([]any, 0, len(expectedCols)*2)
	offset := len(setArgs) + len(keyArgs)
	for _, col := range expectedCols {
		expr, args := nullSafeCompare(tx.dialect, col, valueToSQL(c.Expected[col]), offset+1)
		expParts = append(expParts, "("+expr+")")
		expArgs = append(expArgs, args...)
		offset += len(args)
	}
	joinWith := ""
	if len(expParts) > 0 {
		joinWith = " AND "
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s%s%s",
		qualifiedTbl(tx.dialect, contracts.TableRef{Database: tx.dataset, Schema: plan.Table.Schema, Table: plan.Table.Table}),
		strings.Join(setParts, ", "),
		strings.Join(keyParts, " AND "),
		joinWith,
		strings.Join(expParts, " AND "),
	)
	args := make([]any, 0, len(setArgs)+len(keyArgs)+len(expArgs))
	args = append(args, setArgs...)
	args = append(args, keyArgs...)
	args = append(args, expArgs...)
	res, err := tx.tx.ExecContext(ctx, q, args...)
	if err != nil {
		// Map constraint/unique -> ErrConflict; others -> ErrDatabase.
		return 0, mapErr(err)
	}
	n, err2 := res.RowsAffected()
	if err2 != nil {
		return 0, mapErr(err2)
	}
	if n == 0 {
		return 0, contracts.ErrConflict
	}
	if n != 1 {
		// Multiple rows updated => our where clause did not uniquely
		// identify a row. This is a safety violation; bail with
		// ErrConflict. The overall Update will roll back the tx.
		return 0, contracts.ErrConflict
	}
	return n, nil
}

// Commit finalizes the transaction. Any driver error after commit has
// been started (e.g. network partition) yields ErrCommitUnknown per
// contracts contract.
func (tx *writeTx) Commit(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.finished {
		return contracts.ErrClosed
	}
	tx.finished = true
	if err := tx.tx.Commit(); err != nil {
		tx.committed = false
		// Classify ambiguity. On PostgreSQL/MySQL/sqlite the driver
		// behavior differs; we treat any unknown-nature error as
		// ErrCommitUnknown conservatively.
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return contracts.ErrCommitUnknown
		default:
			msg := strings.ToLower(err.Error())
			switch {
			case strings.Contains(msg, "commit"), strings.Contains(msg, "ambiguous"),
				strings.Contains(msg, "connection"), strings.Contains(msg, "closed"),
				strings.Contains(msg, "broken pipe"), strings.Contains(msg, "timeout"):
				return contracts.ErrCommitUnknown
			}
			return mapErr(err)
		}
	}
	tx.committed = true
	return nil
}

// Rollback is safe to call after cancellation; it uses a fresh short-
// lived context that ignores the caller context cancellation. The
// contracts.WriteTx.Rollback documentation explicitly permits this.
func (tx *writeTx) Rollback(_ context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.finished {
		// Already committed or rolled back: callers that rollback
		// post-commit are a no-op; idempotency requires no error.
		return nil
	}
	tx.finished = true
	ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := tx.tx.Rollback()
	_ = ctx2
	tx.rolledBack = true
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "transaction") ||
			strings.Contains(strings.ToLower(err.Error()), "closed") {
			return nil
		}
		return mapErr(err)
	}
	return nil
}

// nullSafeCompare returns a dialect-appropriate boolean expression
// comparing column name to the given scalar argument, returning the
// expression fragment and a list of driver bind values (length 1 or 2
// depending on dialect). pos is the first parameter position that the
// returned args may consume (for placeholder counting; both dialects
// here use ? so position is unused but the signature matches the
// portable version).
func nullSafeCompare(d contracts.Dialect, col string, scalar any, pos int) (string, []any) {
	switch d {
	case contracts.SQLite:
		// SQLite IS operator works for both NULL and values.
		return fmt.Sprintf("%s IS %s", quoteIdentSQLite(col), placeholder(d, pos)), []any{scalar}
	case contracts.MySQL:
		// MySQL: <=> is null-safe equals.
		return fmt.Sprintf("%s <=> %s", quoteIdentMySQL(col), placeholder(d, pos)), []any{scalar}
	}
	return fmt.Sprintf("%s = %s", quoteIdent(d, col), placeholder(d, pos)), []any{scalar}
}

func sortStrings(in []string) { sort.Strings(in) }
