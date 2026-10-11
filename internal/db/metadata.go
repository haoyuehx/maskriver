package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func (r *reader) Tables(ctx context.Context, requested []string) ([]contracts.TableRef, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		for s := range r.schemas {
			requested = append(requested, s)
		}
	}
	seen := map[string]bool{}
	out := []contracts.TableRef{}
	for _, schema := range requested {
		if !r.schemas[schema] || seen[schema] {
			return nil, contracts.ErrInvalid
		}
		seen[schema] = true
		var query string
		var args []any
		if r.dialect == contracts.SQLite {
			query = "SELECT name FROM main.sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
		} else {
			query = "SELECT table_name FROM information_schema.tables WHERE table_schema=? AND table_type='BASE TABLE' ORDER BY table_name"
			args = []any{schema}
		}
		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, dbError(err)
		}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				break
			}
			out = append(out, contracts.TableRef{Database: r.dataset, Schema: schema, Table: name})
		}
		if err == nil {
			err = rows.Err()
		}
		closeErr := rows.Close()
		if err != nil {
			return nil, dbError(err)
		}
		if closeErr != nil {
			return nil, contracts.ErrDatabase
		}
	}
	return out, nil
}

func (r *reader) Describe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	if err := r.check(ctx); err != nil {
		return contracts.TableSchema{}, err
	}
	return r.describe(ctx, ref)
}

func (a *adapter) describe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	if err := a.table(ref); err != nil {
		return contracts.TableSchema{}, err
	}
	if err := a.exists(ctx, ref); err != nil {
		return contracts.TableSchema{}, err
	}
	if a.dialect == contracts.SQLite {
		return a.sqliteDescribe(ctx, ref)
	}
	return a.mysqlDescribe(ctx, ref)
}

func (a *adapter) exists(ctx context.Context, ref contracts.TableRef) error {
	var found string
	var err error
	if a.dialect == contracts.SQLite {
		err = a.db.QueryRowContext(ctx, "SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", ref.Table).Scan(&found)
	} else {
		err = a.db.QueryRowContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema=? AND table_name=? AND table_type='BASE TABLE'", ref.Schema, ref.Table).Scan(&found)
	}
	return dbError(err)
}

func (a *adapter) sqliteDescribe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	out := contracts.TableSchema{Ref: ref}
	rows, err := a.db.QueryContext(ctx, "PRAGMA main.table_xinfo("+a.quote(ref.Table)+")")
	if err != nil {
		return out, dbError(err)
	}
	pk := map[int]string{}
	for rows.Next() {
		var cid, notnull, pknum, hidden int
		var name, native string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &native, &notnull, &defaultValue, &pknum, &hidden); err != nil {
			break
		}
		if hidden != 0 {
			err = contracts.ErrUnsupported
			break
		}
		typ, typeErr := sqlType(contracts.SQLite, native, false)
		if typeErr != nil {
			err = typeErr
			break
		}
		typ.Nullable = notnull == 0 && !(pknum > 0 && strings.EqualFold(strings.TrimSpace(native), "INTEGER"))
		out.Columns = append(out.Columns, contracts.Column{Ref: contracts.ColumnRef{TableRef: ref, Column: name}, Type: typ})
		if pknum > 0 {
			pk[pknum] = name
		}
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	for i := 1; i <= len(pk); i++ {
		name, ok := pk[i]
		if !ok {
			return contracts.TableSchema{}, contracts.ErrDatabase
		}
		out.PrimaryKey = append(out.PrimaryKey, name)
	}
	out.Coverage.Columns, out.Coverage.PrimaryKey = true, true

	rows, err = a.db.QueryContext(ctx, "PRAGMA main.index_list("+a.quote(ref.Table)+")")
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	type idxSpec struct {
		name   string
		unique bool
		origin string
	}
	specs := []idxSpec{}
	indexCoverage := true
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err = rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			break
		}
		if partial != 0 {
			indexCoverage = false
			continue
		}
		specs = append(specs, idxSpec{name, unique != 0, origin})
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr = rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	for _, spec := range specs {
		ir, qerr := a.db.QueryContext(ctx, "PRAGMA main.index_xinfo("+a.quote(spec.name)+")")
		if qerr != nil {
			return contracts.TableSchema{}, dbError(qerr)
		}
		cols := map[int]string{}
		for ir.Next() {
			var seqno, cid, desc, key int
			var name, coll sql.NullString
			if qerr = ir.Scan(&seqno, &cid, &name, &desc, &coll, &key); qerr != nil {
				break
			}
			if key == 1 {
				if cid < 0 || !name.Valid {
					qerr = contracts.ErrUnsupported
					break
				}
				cols[seqno] = name.String
			}
		}
		if qerr == nil {
			qerr = ir.Err()
		}
		closeErr = ir.Close()
		if qerr != nil {
			return contracts.TableSchema{}, dbErrorOrCategory(qerr)
		}
		if closeErr != nil {
			return contracts.TableSchema{}, contracts.ErrDatabase
		}
		ordered := make([]string, 0, len(cols))
		for i := 0; i < len(cols); i++ {
			name, ok := cols[i]
			if !ok {
				return contracts.TableSchema{}, contracts.ErrDatabase
			}
			ordered = append(ordered, name)
		}
		out.Indexes = append(out.Indexes, contracts.Index{Name: spec.name, Columns: ordered, Unique: spec.unique})
		if spec.unique && spec.origin != "pk" {
			out.Unique = append(out.Unique, ordered)
		}
	}
	out.Coverage.Indexes, out.Coverage.Unique = indexCoverage, indexCoverage

	fr, err := a.db.QueryContext(ctx, "PRAGMA main.foreign_key_list("+a.quote(ref.Table)+")")
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	fkByID := map[int]*contracts.ForeignKey{}
	maxID := -1
	for fr.Next() {
		var id, seq int
		var target, from, to, onUpdate, onDelete, match string
		if err = fr.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			break
		}
		if id > maxID {
			maxID = id
		}
		fk := fkByID[id]
		if fk == nil {
			fk = &contracts.ForeignKey{Target: contracts.TableRef{Database: a.dataset, Schema: ref.Schema, Table: target}, OnUpdate: onUpdate, OnDelete: onDelete}
			fkByID[id] = fk
		}
		if seq != len(fk.Columns) {
			err = contracts.ErrUnsupported
			break
		}
		fk.Columns = append(fk.Columns, from)
		fk.TargetColumns = append(fk.TargetColumns, to)
	}
	if err == nil {
		err = fr.Err()
	}
	closeErr = fr.Close()
	if err != nil {
		return contracts.TableSchema{}, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	for id := 0; id <= maxID; id++ {
		fk, ok := fkByID[id]
		if !ok {
			return contracts.TableSchema{}, contracts.ErrDatabase
		}
		out.ForeignKeys = append(out.ForeignKeys, *fk)
	}
	out.Coverage.ForeignKeys = true
	// SQLite does not expose structured CHECK expressions through PRAGMA.
	// False records that we have not safely parsed all of them.
	out.Coverage.Checks = false
	return out, nil
}

func (a *adapter) mysqlDescribe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	out := contracts.TableSchema{Ref: ref}
	rows, err := a.db.QueryContext(ctx, "SELECT column_name,data_type,column_type,is_nullable,character_maximum_length,numeric_precision,numeric_scale,collation_name FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position", ref.Schema, ref.Table)
	if err != nil {
		return out, dbError(err)
	}
	for rows.Next() {
		var name, dataType, native, nullable string
		var length, precision, scale sql.NullInt64
		var collation sql.NullString
		if err = rows.Scan(&name, &dataType, &native, &nullable, &length, &precision, &scale, &collation); err != nil {
			break
		}
		typ, typeErr := sqlType(contracts.MySQL, dataType, strings.Contains(strings.ToLower(native), "unsigned"))
		if typeErr != nil {
			err = typeErr
			break
		}
		typ.Native = native
		typ.Nullable = nullable == "YES"
		if length.Valid {
			typ.Length = int(length.Int64)
		}
		if precision.Valid {
			typ.Precision = int(precision.Int64)
		}
		if scale.Valid {
			typ.Scale = int(scale.Int64)
		}
		if collation.Valid {
			typ.Collation = collation.String
		}
		out.Columns = append(out.Columns, contracts.Column{Ref: contracts.ColumnRef{TableRef: ref, Column: name}, Type: typ})
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	out.Coverage.Columns = true

	rows, err = a.db.QueryContext(ctx, "SELECT column_name FROM information_schema.key_column_usage WHERE table_schema=? AND table_name=? AND constraint_name='PRIMARY' ORDER BY ordinal_position", ref.Schema, ref.Table)
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			break
		}
		out.PrimaryKey = append(out.PrimaryKey, name)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr = rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	out.Coverage.PrimaryKey = true

	rows, err = a.db.QueryContext(ctx, "SELECT index_name,column_name,non_unique,seq_in_index FROM information_schema.statistics WHERE table_schema=? AND table_name=? ORDER BY index_name,seq_in_index", ref.Schema, ref.Table)
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	byName := map[string]*contracts.Index{}
	order := []string{}
	for rows.Next() {
		var name string
		var col sql.NullString
		var nonUnique, seq int
		if err = rows.Scan(&name, &col, &nonUnique, &seq); err != nil {
			break
		}
		if !col.Valid {
			err = contracts.ErrUnsupported
			break
		}
		idx := byName[name]
		if idx == nil {
			idx = &contracts.Index{Name: name, Unique: nonUnique == 0}
			byName[name] = idx
			order = append(order, name)
		}
		if seq != len(idx.Columns)+1 {
			err = contracts.ErrDatabase
			break
		}
		idx.Columns = append(idx.Columns, col.String)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr = rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	for _, name := range order {
		idx := *byName[name]
		out.Indexes = append(out.Indexes, idx)
		if idx.Unique && name != "PRIMARY" {
			out.Unique = append(out.Unique, append([]string(nil), idx.Columns...))
		}
	}
	out.Coverage.Indexes, out.Coverage.Unique = true, true

	rows, err = a.db.QueryContext(ctx, "SELECT k.constraint_name,k.column_name,k.referenced_table_schema,k.referenced_table_name,k.referenced_column_name,r.update_rule,r.delete_rule,k.ordinal_position FROM information_schema.key_column_usage k JOIN information_schema.referential_constraints r ON r.constraint_schema=k.constraint_schema AND r.constraint_name=k.constraint_name AND r.table_name=k.table_name WHERE k.table_schema=? AND k.table_name=? AND k.referenced_table_name IS NOT NULL ORDER BY k.constraint_name,k.ordinal_position", ref.Schema, ref.Table)
	if err != nil {
		return contracts.TableSchema{}, dbError(err)
	}
	fkMap := map[string]*contracts.ForeignKey{}
	fkOrder := []string{}
	for rows.Next() {
		var name, col, targetSchema, targetTable, targetCol, onUpdate, onDelete string
		var ordinal int
		if err = rows.Scan(&name, &col, &targetSchema, &targetTable, &targetCol, &onUpdate, &onDelete, &ordinal); err != nil {
			break
		}
		fk := fkMap[name]
		if fk == nil {
			fk = &contracts.ForeignKey{Target: contracts.TableRef{Database: a.dataset, Schema: targetSchema, Table: targetTable}, OnUpdate: onUpdate, OnDelete: onDelete}
			fkMap[name] = fk
			fkOrder = append(fkOrder, name)
		}
		if ordinal != len(fk.Columns)+1 {
			err = contracts.ErrDatabase
			break
		}
		fk.Columns = append(fk.Columns, col)
		fk.TargetColumns = append(fk.TargetColumns, targetCol)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr = rows.Close()
	if err != nil {
		return contracts.TableSchema{}, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return contracts.TableSchema{}, contracts.ErrDatabase
	}
	for _, name := range fkOrder {
		out.ForeignKeys = append(out.ForeignKeys, *fkMap[name])
	}
	out.Coverage.ForeignKeys = true

	rows, err = a.db.QueryContext(ctx, "SELECT c.check_clause FROM information_schema.table_constraints t JOIN information_schema.check_constraints c ON t.constraint_schema=c.constraint_schema AND t.constraint_name=c.constraint_name WHERE t.table_schema=? AND t.table_name=? AND t.constraint_type='CHECK' ORDER BY t.constraint_name", ref.Schema, ref.Table)
	if err == nil {
		for rows.Next() {
			var clause string
			if err = rows.Scan(&clause); err != nil {
				break
			}
			out.Checks = append(out.Checks, clause)
		}
		if err == nil {
			err = rows.Err()
		}
		closeErr = rows.Close()
		if err == nil && closeErr == nil {
			out.Coverage.Checks = true
		} else if err != nil {
			return contracts.TableSchema{}, dbError(err)
		}
	}
	return out, nil
}

func sqlType(d contracts.Dialect, native string, unsigned bool) (contracts.Type, error) {
	n := strings.ToLower(strings.TrimSpace(native))
	t := contracts.Type{Native: native}
	if d == contracts.SQLite {
		if strings.Contains(n, "unsigned") {
			return t, contracts.ErrUnsupported
		}
		switch {
		case strings.Contains(n, "int"):
			t.Kind = contracts.Int
		case strings.Contains(n, "blob"):
			t.Kind = contracts.Bytes
		case strings.Contains(n, "date") && !strings.Contains(n, "time"):
			t.Kind = contracts.Date
		case strings.Contains(n, "datetime"):
			t.Kind = contracts.LocalDateTime
		case strings.Contains(n, "char"), strings.Contains(n, "text"), strings.Contains(n, "clob"):
			t.Kind = contracts.Text
		case strings.Contains(n, "real"), strings.Contains(n, "floa"), strings.Contains(n, "doub"):
			t.Kind = contracts.Float
		case strings.Contains(n, "dec"):
			return t, contracts.ErrUnsupported // SQLite NUMERIC affinity can discard decimal scale.
		case strings.Contains(n, "bool"):
			t.Kind = contracts.Bool
		default:
			return t, contracts.ErrUnsupported
		}
	} else {
		switch n {
		case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
			t.Kind = contracts.Text
		case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
			t.Kind = contracts.Bytes
		case "tinyint", "smallint", "mediumint", "int", "integer", "bigint":
			if unsigned {
				t.Kind = contracts.Uint
			} else {
				t.Kind = contracts.Int
			}
		case "float", "double":
			t.Kind = contracts.Float
		case "decimal", "numeric":
			t.Kind = contracts.Decimal
		case "date":
			t.Kind = contracts.Date
		case "datetime":
			t.Kind = contracts.LocalDateTime
		default:
			return t, contracts.ErrUnsupported
		}
	}
	return t, nil
}

func dbErrorOrCategory(err error) error {
	if err == contracts.ErrUnsupported || err == contracts.ErrInvalid || err == contracts.ErrDatabase {
		return err
	}
	return dbError(err)
}
