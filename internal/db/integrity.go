package db

import (
	"context"
	"strings"

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
		return r.mysqlIntegrityCheck(ctx, ref)
	default:
		return contracts.ErrUnsupported
	}
}

// mysqlIntegrityCheck runs the physical table check and an explicit orphan-row
// scan for every foreign key that touches the planned table. CHECK TABLE alone
// is not enough: it validates storage, while referential integrity is a logical
// property. Every returned CHECK TABLE row must be a plain "status"/"OK"; any
// warning/note/error row or an empty result fails closed.
func (r *reader) mysqlIntegrityCheck(ctx context.Context, ref contracts.TableRef) error {
	var foreignKeyChecks int
	if err := r.db.QueryRowContext(ctx, "SELECT @@SESSION.foreign_key_checks").Scan(&foreignKeyChecks); err != nil {
		return dbError(err)
	}
	if foreignKeyChecks != 1 {
		return contracts.ErrConflict
	}
	rows, err := r.db.QueryContext(ctx, "CHECK TABLE "+r.qualified(ref)+" QUICK")
	if err != nil {
		return dbError(err)
	}
	count := 0
	for rows.Next() {
		var table, op, msgType, msgText string
		if err = rows.Scan(&table, &op, &msgType, &msgText); err != nil {
			break
		}
		count++
		if !strings.EqualFold(strings.TrimSpace(msgType), "status") ||
			!strings.EqualFold(strings.TrimSpace(msgText), "OK") {
			err = contracts.ErrConflict
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return dbErrorOrCategory(err)
	}
	if closeErr != nil || count == 0 {
		return contracts.ErrDatabase
	}
	return r.mysqlForeignKeyOrphans(ctx, ref)
}

type mysqlForeignKey struct {
	schema, table, column     string
	targetSchema              string
	targetTable, targetColumn string
}

// mysqlForeignKeyOrphans counts rows whose non-NULL child columns have no
// matching parent row for every FK where ref is the child or the parent table.
// It is intentionally database-schema scoped; a parent in another schema is
// queried by its real name and a privilege failure is a fail-closed database
// error, not a silent skip.
func (r *reader) mysqlForeignKeyOrphans(ctx context.Context, ref contracts.TableRef) error {
	rows, err := r.db.QueryContext(ctx, `SELECT k.constraint_name,k.table_schema,k.table_name,k.column_name,
		k.referenced_table_schema,k.referenced_table_name,k.referenced_column_name
		FROM information_schema.key_column_usage k
		WHERE k.referenced_table_name IS NOT NULL
		  AND ((k.table_schema=? AND k.table_name=?) OR (k.referenced_table_schema=? AND k.referenced_table_name=?))
		ORDER BY k.constraint_name,k.ordinal_position`, r.schema, ref.Table, r.schema, ref.Table)
	if err != nil {
		return dbError(err)
	}
	type key struct{ schema, name string }
	grouped := map[key][]mysqlForeignKey{}
	order := []key{}
	for rows.Next() {
		var constraintName string
		var fk mysqlForeignKey
		if err = rows.Scan(&constraintName, &fk.schema, &fk.table, &fk.column,
			&fk.targetSchema, &fk.targetTable, &fk.targetColumn); err != nil {
			break
		}
		k := key{fk.schema, constraintName}
		if _, ok := grouped[k]; !ok {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], fk)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return dbError(err)
	}
	if closeErr != nil {
		return contracts.ErrDatabase
	}
	for _, k := range order {
		parts := grouped[k]
		if len(parts) == 0 || len(parts[0].targetTable) == 0 {
			return contracts.ErrIncomplete
		}
		join := make([]string, 0, len(parts))
		nonNull := make([]string, 0, len(parts))
		for _, part := range parts {
			join = append(join, "c."+r.quote(part.column)+" = p."+r.quote(part.targetColumn))
			nonNull = append(nonNull, "c."+r.quote(part.column)+" IS NOT NULL")
		}
		query := "SELECT COUNT(*) FROM " + r.quote(parts[0].schema) + "." + r.quote(parts[0].table) +
			" c LEFT JOIN " + r.quote(parts[0].targetSchema) + "." + r.quote(parts[0].targetTable) +
			" p ON " + strings.Join(join, " AND ") +
			" WHERE " + strings.Join(nonNull, " AND ") +
			" AND p." + r.quote(parts[0].targetColumn) + " IS NULL"
		var orphans int64
		if err := r.db.QueryRowContext(ctx, query).Scan(&orphans); err != nil {
			return dbError(err)
		}
		if orphans != 0 {
			return contracts.ErrConflict
		}
	}
	return nil
}
