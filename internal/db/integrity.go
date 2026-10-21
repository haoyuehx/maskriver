package db

import (
	"context"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// IntegrityCheck is the strict-validation database capability. It checks the
// physical/logical integrity of one planned table on this snapshot. It is not
// part of the frozen contracts.Reader interface; verify treats a reader without
// it as unsupported in strict mode.
func (r *reader) IntegrityCheck(ctx context.Context, ref contracts.TableRef) error {
	if err := r.check(ctx); err != nil {
		return err
	}
	if err := r.table(ref); err != nil {
		return err
	}
	if err := r.exists(ctx, ref); err != nil {
		return err
	}
	switch r.dialect {
	case contracts.SQLite:
		var result string
		if err := r.db.QueryRowContext(ctx, "PRAGMA main.quick_check").Scan(&result); err != nil {
			return dbError(err)
		}
		if result != "ok" {
			return contracts.ErrConflict
		}
		// Database-wide: checking only the planned table misses children that
		// reference it. Any returned row is a violation.
		rows, err := r.db.QueryContext(ctx, "PRAGMA main.foreign_key_check")
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		if rows.Next() {
			return contracts.ErrConflict
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		return nil
	case contracts.MySQL:
		// MySQL strict integrity is not implemented in M3-1. CHECK TABLE only
		// covers physical storage, not FK/CHECK semantics, and was not verified
		// against the isolated MySQL CI instance. Fail closed instead of shipping
		// a partial gate; MySQL CLI/recovery remain separate work.
		return contracts.ErrUnsupported
	default:
		return contracts.ErrUnsupported
	}
}
