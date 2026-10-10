package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// -------- Reader implementation (shared by SQLite and MySQL) ----------

func (b *baseHandle) Tables(ctx context.Context, schemas []string) ([]contracts.TableRef, error) {
	if err := b.assertOpen(); err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		schemas = b.schemas
	}
	switch b.dialect {
	case contracts.SQLite:
		return b.sqliteTables(ctx, schemas)
	case contracts.MySQL:
		return b.mysqlTables(ctx, schemas)
	}
	return nil, contracts.ErrUnsupported
}

func (b *baseHandle) sqliteTables(ctx context.Context, schemas []string) ([]contracts.TableRef, error) {
	attached := make([]string, 0, 2+len(schemas))
	seen := map[string]struct{}{}
	// Always include "main" exactly once.
	for _, s := range append([]string{"main"}, schemas...) {
		if s == "" {
			s = "main"
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		attached = append(attached, s)
	}
	out := make([]contracts.TableRef, 0, 32)
	for _, s := range attached {
		if s == "" {
			s = "main"
		}
		q := fmt.Sprintf("SELECT name FROM %s.sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%%' ORDER BY name", quoteIdentSQLite(s))
		rows, err := b.db.QueryContext(ctx, q)
		if err != nil {
			return nil, mapErr(err)
		}
		for rows.Next() {
			var name string
			if err2 := rows.Scan(&name); err2 != nil {
				rows.Close()
				return nil, mapErr(err2)
			}
			out = append(out, contracts.TableRef{Database: b.dataset, Schema: s, Table: name})
		}
		cerr := rows.Close()
		if cerr != nil {
			return nil, mapErr(cerr)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Schema != out[j].Schema {
			return out[i].Schema < out[j].Schema
		}
		return out[i].Table < out[j].Table
	})
	return out, nil
}

func (b *baseHandle) mysqlTables(ctx context.Context, schemas []string) ([]contracts.TableRef, error) {
	if len(schemas) == 0 {
		// SELECT DATABASE() as fallback.
		var curDB sql.NullString
		if err := b.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&curDB); err == nil && curDB.Valid {
			schemas = []string{curDB.String}
		}
	}
	args := make([]any, 0, len(schemas))
	ph := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if s == "" {
			continue
		}
		ph = append(ph, "?")
		args = append(args, s)
	}
	q := "SELECT table_schema, table_name FROM information_schema.tables WHERE table_type='BASE TABLE'"
	if len(ph) > 0 {
		q += " AND table_schema IN (" + strings.Join(ph, ",") + ")"
	}
	q += " ORDER BY table_schema, table_name"
	rows, err := b.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]contracts.TableRef, 0, 64)
	for rows.Next() {
		var schema, table string
		if err2 := rows.Scan(&schema, &table); err2 != nil {
			return nil, mapErr(err2)
		}
		out = append(out, contracts.TableRef{Database: b.dataset, Schema: schema, Table: table})
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

func (b *baseHandle) Describe(ctx context.Context, t contracts.TableRef) (contracts.TableSchema, error) {
	if err := b.assertOpen(); err != nil {
		return contracts.TableSchema{}, err
	}
	if t.Table == "" {
		return contracts.TableSchema{}, contracts.ErrInvalid
	}
	switch b.dialect {
	case contracts.SQLite:
		return b.sqliteDescribe(ctx, t)
	case contracts.MySQL:
		return b.mysqlDescribe(ctx, t)
	}
	return contracts.TableSchema{}, contracts.ErrUnsupported
}

func (b *baseHandle) sqliteDescribe(ctx context.Context, t contracts.TableRef) (contracts.TableSchema, error) {
	schema := t.Schema
	if schema == "" {
		schema = "main"
	}
	q := fmt.Sprintf("PRAGMA %s.table_info(%s)", quoteIdentSQLite(schema), quoteIdentSQLite(t.Table))
	rows, err := b.db.QueryContext(ctx, q)
	if err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	defer rows.Close()
	cols := make([]contracts.Column, 0, 16)
	pkSet := make([]string, 0, 4)
	// pragma schema: cid, name, type, notnull, dflt_value, pk.
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err2 := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err2 != nil {
			return contracts.TableSchema{}, mapErr(err2)
		}
		kind := mapNativeKind(strings.ToUpper(typ), name, strings.ToUpper(typ))
		col := contracts.Column{
			Ref: contracts.ColumnRef{TableRef: contracts.TableRef{Database: b.dataset, Schema: schema, Table: t.Table}, Column: name},
			Type: contracts.Type{
				Kind:     kind,
				Native:   typ,
				Nullable: notnull == 0,
			},
		}
		cols = append(cols, col)
		if pk > 0 {
			pkSet = append(pkSet, "") // placeholder; replaced after sort by pk order
		}
	}
	if err := rows.Err(); err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	_ = rows.Close()
	// For SQLite we must re-read pragma to preserve pk order since each row pk is its key order.
	rows2, err := b.db.QueryContext(ctx, q)
	if err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	type pkRow struct {
		name string
		ord  int
	}
	pks := make([]pkRow, 0, 4)
	for rows2.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err2 := rows2.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err2 != nil {
			rows2.Close()
			return contracts.TableSchema{}, mapErr(err2)
		}
		if pk > 0 {
			pks = append(pks, pkRow{name: name, ord: pk})
		}
	}
	rows2.Close()
	sort.SliceStable(pks, func(i, j int) bool { return pks[i].ord < pks[j].ord })
	pkNames := make([]string, 0, len(pks))
	for _, p := range pks {
		pkNames = append(pkNames, p.name)
	}
	return contracts.TableSchema{
		Ref:        contracts.TableRef{Database: b.dataset, Schema: schema, Table: t.Table},
		Columns:    cols,
		PrimaryKey: pkNames,
		Coverage: contracts.SchemaCoverage{
			Columns: true, PrimaryKey: len(pkNames) > 0 || (len(cols) > 0),
			Indexes: false, ForeignKeys: false, Unique: false, Checks: false,
		},
	}, nil
}

func (b *baseHandle) mysqlDescribe(ctx context.Context, t contracts.TableRef) (contracts.TableSchema, error) {
	schema := t.Schema
	if schema == "" {
		var curDB sql.NullString
		if err := b.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&curDB); err != nil {
			return contracts.TableSchema{}, mapErr(err)
		}
		if !curDB.Valid {
			return contracts.TableSchema{}, contracts.ErrInvalid
		}
		schema = curDB.String
	}
	q := `SELECT c.column_name, c.data_type, c.column_type, c.is_nullable, c.character_maximum_length, c.numeric_precision, c.numeric_scale
		FROM information_schema.columns c
		WHERE c.table_schema = ? AND c.table_name = ?
		ORDER BY c.ordinal_position`
	rows, err := b.db.QueryContext(ctx, q, schema, t.Table)
	if err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	defer rows.Close()
	cols := make([]contracts.Column, 0, 16)
	for rows.Next() {
		var colName, dataType, colType, nullable string
		var charLen, numPrec, numScale sql.NullInt64
		if err2 := rows.Scan(&colName, &dataType, &colType, &nullable, &charLen, &numPrec, &numScale); err2 != nil {
			return contracts.TableSchema{}, mapErr(err2)
		}
		kind := mapNativeKind(strings.ToUpper(dataType), colName, strings.ToUpper(colType))
		c := contracts.Column{
			Ref:  contracts.ColumnRef{TableRef: contracts.TableRef{Database: b.dataset, Schema: schema, Table: t.Table}, Column: colName},
			Type: contracts.Type{Kind: kind, Native: colType, Nullable: nullable == "YES"},
		}
		if charLen.Valid {
			c.Type.Length = int(charLen.Int64)
		}
		if numPrec.Valid {
			c.Type.Precision = int(numPrec.Int64)
		}
		if numScale.Valid {
			c.Type.Scale = int(numScale.Int64)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	// Primary key.
	qpk := `SELECT k.column_name FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage k
		  ON tc.constraint_schema = k.constraint_schema AND tc.constraint_name = k.constraint_name AND tc.table_name = k.table_name
		WHERE tc.constraint_type='PRIMARY KEY' AND tc.table_schema=? AND tc.table_name=?
		ORDER BY k.ordinal_position`
	pkRows, err := b.db.QueryContext(ctx, qpk, schema, t.Table)
	if err != nil {
		return contracts.TableSchema{}, mapErr(err)
	}
	defer pkRows.Close()
	pks := make([]string, 0, 4)
	for pkRows.Next() {
		var n string
		if err2 := pkRows.Scan(&n); err2 != nil {
			return contracts.TableSchema{}, mapErr(err2)
		}
		pks = append(pks, n)
	}
	return contracts.TableSchema{
		Ref:        contracts.TableRef{Database: b.dataset, Schema: schema, Table: t.Table},
		Columns:    cols,
		PrimaryKey: pks,
		Coverage: contracts.SchemaCoverage{
			Columns: true, PrimaryKey: true, Indexes: false, ForeignKeys: false, Unique: false, Checks: false,
		},
	}, nil
}

// Sample returns DistinctNonNull values for a single column, bounded by
// SampleRequest.Limit. Truncated=true if more than Limit distinct
// non-NULL values exist. Basis is always DistinctNonNull or BasisUnknown
// on error-return paths.
func (b *baseHandle) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	out := contracts.Sample{Basis: contracts.BasisUnknown}
	if err := b.assertOpen(); err != nil {
		return out, err
	}
	if req.Column.Column == "" || req.Limit <= 0 {
		return out, contracts.ErrInvalid
	}
	// ColumnRef.TableRef is optional for Sample; when absent the
	// adapter treats Schema="" as default.
	table := req.Column.TableRef
	if table.Table == "" {
		return out, contracts.ErrInvalid
	}
	// Fetch up to Limit+1 so Truncated is authoritative.
	limit := req.Limit
	if limit > 100000 {
		limit = 100000
	}
	want := limit + 1
	q := fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL LIMIT %d",
		colList(b.dialect, []string{req.Column.Column}),
		qualifiedTbl(b.dialect, table),
		quoteIdent(b.dialect, req.Column.Column),
		want,
	)
	rows, err := b.db.QueryContext(ctx, q)
	if err != nil {
		return out, mapErr(err)
	}
	defer rows.Close()
	vals := make([]contracts.Value, 0, limit)
	for rows.Next() {
		var raw sql.NullString
		if err2 := rows.Scan(&raw); err2 != nil {
			return out, mapErr(err2)
		}
		if !raw.Valid {
			continue
		}
		// Use the kind-inferring helper; text is always safe (the
		// Value encoding documentation permits this when the caller
		// uses the DistinctNonNull sample basis).
		v, err2 := contracts.NewValue(contracts.Text, raw.String)
		if err2 != nil {
			// Should not occur for text; skip conservatively.
			continue
		}
		if len(vals) >= limit {
			vals = append(vals, v)
			break
		}
		vals = append(vals, v)
	}
	if err := rows.Err(); err != nil {
		return out, mapErr(err)
	}
	truncated := len(vals) > limit
	if truncated {
		vals = vals[:limit]
	}
	return contracts.Sample{Values: vals, Basis: contracts.DistinctNonNull, Truncated: truncated}, nil
}

// Count returns an exact row count for the table. COUNT(*) is
// authoritative; for large tables this may be slow, which matches the
// contracts expectation that Count is used only for metadata.
func (b *baseHandle) Count(ctx context.Context, t contracts.TableRef) (int64, error) {
	if err := b.assertOpen(); err != nil {
		return 0, err
	}
	if t.Table == "" {
		return 0, contracts.ErrInvalid
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s", qualifiedTbl(b.dialect, t))
	var n int64
	if err := b.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

// Close is idempotent.
func (b *baseHandle) Close() error { return b.closeLocked() }

func quoteIdent(d contracts.Dialect, s string) string {
	if d == contracts.MySQL {
		return quoteIdentMySQL(s)
	}
	return quoteIdentSQLite(s)
}

// mapNativeKind returns the closest contracts.Kind for a column type.
// Unknown types fall back to Bytes so that value pass-through is not
// silently coerced to Text (avoids UTF-8 assertions firing for
// binary-looking native types).
func mapNativeKind(dataTypeUpper, colName, colTypeUpper string) contracts.Kind {
	switch {
	case strings.HasPrefix(dataTypeUpper, "INT") || strings.HasPrefix(dataTypeUpper, "BIGINT") ||
		strings.HasPrefix(dataTypeUpper, "SMALLINT") || strings.HasPrefix(dataTypeUpper, "TINYINT") ||
		dataTypeUpper == "INTEGER" || strings.HasPrefix(dataTypeUpper, "MEDIUMINT"):
		if strings.Contains(colTypeUpper, "UNSIGNED") {
			return contracts.Uint
		}
		return contracts.Int
	case strings.HasPrefix(dataTypeUpper, "FLOAT") || strings.HasPrefix(dataTypeUpper, "DOUBLE") ||
		strings.HasPrefix(dataTypeUpper, "REAL"):
		return contracts.Float
	case strings.HasPrefix(dataTypeUpper, "DECIMAL") || strings.HasPrefix(dataTypeUpper, "NUMERIC"):
		return contracts.Decimal
	case strings.HasPrefix(dataTypeUpper, "BOOL") || dataTypeUpper == "BOOLEAN":
		return contracts.Bool
	case dataTypeUpper == "DATE":
		return contracts.Date
	case strings.HasPrefix(dataTypeUpper, "DATETIME") || strings.HasPrefix(dataTypeUpper, "TIMESTAMP"):
		return contracts.LocalDateTime
	case dataTypeUpper == "INSTANT":
		return contracts.Instant
	case strings.HasPrefix(dataTypeUpper, "VARCHAR") || strings.HasPrefix(dataTypeUpper, "CHAR") ||
		dataTypeUpper == "TEXT" || strings.HasPrefix(dataTypeUpper, "TINYTEXT") ||
		strings.HasPrefix(dataTypeUpper, "MEDIUMTEXT") || strings.HasPrefix(dataTypeUpper, "LONGTEXT") ||
		strings.HasPrefix(dataTypeUpper, "NVARCHAR") || strings.HasPrefix(dataTypeUpper, "CLOB"):
		return contracts.Text
	case strings.HasPrefix(dataTypeUpper, "BLOB") || strings.HasPrefix(dataTypeUpper, "BINARY") ||
		strings.HasPrefix(dataTypeUpper, "VARBINARY") || strings.HasPrefix(dataTypeUpper, "BYTEA") ||
		dataTypeUpper == "RAW":
		return contracts.Bytes
	}
	return contracts.Bytes
}

// --------- keyset paging --------------------------------------------

// Page implements keyset pagination without OFFSET. Requires KeyColumns
// matching the declared primary key (exact list, order matters). When
// KeyColumns is empty, ErrUnsupported is returned (the runner must
// always pass a keyed PageRequest). After is compared tuple-wise in
// KeyColumns order using strict lexicographic > with NULL-safe column
// comparison.
func (b *baseHandle) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	empty := contracts.Page{}
	if err := b.assertOpen(); err != nil {
		return empty, err
	}
	if req.Table.Table == "" {
		return empty, contracts.ErrInvalid
	}
	if len(req.KeyColumns) == 0 {
		return empty, contracts.ErrUnsupported
	}
	if req.Limit <= 0 {
		return empty, contracts.ErrInvalid
	}
	if len(req.Columns) == 0 {
		return empty, contracts.ErrInvalid
	}
	if len(req.After) > 0 && len(req.After) != len(req.KeyColumns) {
		return empty, contracts.ErrInvalid
	}
	// Verify no duplicate columns and that key columns are represented.
	colSet := make(map[string]struct{}, len(req.Columns))
	for _, c := range req.Columns {
		if _, ok := colSet[c]; ok {
			return empty, contracts.ErrInvalid
		}
		colSet[c] = struct{}{}
	}
	for _, k := range req.KeyColumns {
		if _, ok := colSet[k]; !ok {
			return empty, contracts.ErrInvalid
		}
	}
	// Fetch the declared column Kind metadata for key columns so we
	// can cast After values correctly. This lets the adapter accept
	// Values encoded as Text from previous-page cursors (which is the
	// Page output contract) while still binding them as integers etc.
	// when talking to the driver. We tolerate Describe failure only
	// when the caller has explicitly passed zero-length After (no
	// cursor), and skip the cast otherwise (mapping driver type-coerce
	// errors to ErrInvalid).
	keyKinds := make([]contracts.Kind, len(req.KeyColumns))
	for i := range keyKinds {
		keyKinds[i] = contracts.Null
	}
	if len(req.After) > 0 {
		sch, err := b.Describe(ctx, req.Table)
		if err != nil {
			return empty, mapErr(err)
		}
		kindByName := make(map[string]contracts.Kind, len(sch.Columns))
		for _, c := range sch.Columns {
			kindByName[c.Ref.Column] = c.Type.Kind
		}
		for i, k := range req.KeyColumns {
			if kd, ok := kindByName[k]; ok {
				keyKinds[i] = kd
			}
		}
	}

	// Build WHERE clause for tuple comparison: (k1, k2, k3) > (a1, a2, a3).
	// Dialect-specific strict greater-than and equality operators each
	// emit ONE ? placeholder and ONE arg per key column. SQLite's `IS`
	// and MySQL's `<=>` both accept placeholders exactly like regular
	// =, so we get one arg per key column per predicate operator.
	where := ""
	args := make([]any, 0, len(req.KeyColumns)*2)
	if len(req.After) > 0 {
		preds := make([]string, 0, len(req.KeyColumns))
		var prevEq []string
		pos := 0
		for i, k := range req.KeyColumns {
			want := keyKinds[i]
			raw := req.After[i]
			scalar := valueToSQL(raw)
			if want != contracts.Null && want != raw.Kind() && !raw.IsNull() {
				cast, errC := contracts.NewValue(want, raw.Payload())
				if errC != nil {
					return empty, contracts.ErrInvalid
				}
				scalar = valueToSQL(cast)
			}
			// Strict greater-than predicate: new placeholder.
			pos++
			greater := fmt.Sprintf("%s > %s", quoteIdent(b.dialect, k), placeholder(b.dialect, pos))
			andPrev := strings.Join(prevEq, " AND ")
			piece := greater
			if andPrev != "" {
				piece = andPrev + " AND " + greater
			}
			preds = append(preds, "("+piece+")")
			args = append(args, scalar)
			// Equality predicate: another placeholder (also binds the
			// same scalar). SQLite IS and MySQL <=> both accept ?.
			pos++
			eqExpr := fmt.Sprintf("%s %s %s",
				quoteIdent(b.dialect, k),
				nullSafeEqualOperator(b.dialect),
				placeholder(b.dialect, pos),
			)
			prevEq = append(prevEq, eqExpr)
			args = append(args, scalar)
		}
		where = "WHERE " + strings.Join(preds, " OR ")
	}

	// ORDER BY key columns then SELECT columns, LIMIT Limit+1.
	orderParts := make([]string, 0, len(req.KeyColumns))
	for _, k := range req.KeyColumns {
		orderParts = append(orderParts, quoteIdent(b.dialect, k)+" ASC")
	}
	q := fmt.Sprintf(
		"SELECT %s FROM %s %s ORDER BY %s LIMIT %d",
		colList(b.dialect, req.Columns),
		qualifiedTbl(b.dialect, req.Table),
		where,
		strings.Join(orderParts, ", "),
		req.Limit+1,
	)
	_ = 0
	rows, err := b.db.QueryContext(ctx, q, args...)
	if err != nil {
		return empty, mapErr(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return empty, mapErr(err)
	}
	// Column order from driver must match req.Columns within textual alias.
	// We validate by count, then map by lowercased name.
	if len(cols) != len(req.Columns) {
		return empty, contracts.ErrInvalid
	}
	idxByName := make(map[string]int, len(cols))
	for i, c := range cols {
		idxByName[strings.ToLower(c)] = i
	}
	keyIdx := make([]int, 0, len(req.KeyColumns))
	for _, k := range req.KeyColumns {
		idx, ok := idxByName[strings.ToLower(k)]
		if !ok {
			return empty, contracts.ErrInvalid
		}
		keyIdx = append(keyIdx, idx)
	}
	result := make([]contracts.Row, 0, req.Limit)
	// Accumulate rows up to Limit+1; the (Limit+1)-th row is used only
	// to compute the Next cursor (keyset continuation marker), and it
	// is NOT returned in Rows.
	collected := make([]contracts.Row, 0, req.Limit+1)
	rawScans := make([][]any, 0, req.Limit+1)
	for rows.Next() {
		scans := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range scans {
			ptrs[i] = &scans[i]
		}
		if err2 := rows.Scan(ptrs...); err2 != nil {
			return empty, mapErr(err2)
		}
		row := make(contracts.Row, len(cols))
		for i, col := range req.Columns {
			v, err2 := sqlToValue(scans[i])
			if err2 != nil {
				return empty, mapErr(err2)
			}
			row[col] = v
		}
		collected = append(collected, row)
		rawScans = append(rawScans, scans)
	}
	if err := rows.Err(); err != nil {
		return empty, mapErr(err)
	}
	if len(collected) > req.Limit {
		// More rows exist: return up to req.Limit rows, and use the
		// last-returned-row's key columns as the keyset cursor.
		// Using the last returned row as the cursor (instead of the
		// sentinel row at Limit index) ensures boundary rows with
		// equal-prefix keys are not skipped when the caller passes
		// the cursor back through the strict greater-than predicate.
		result = collected[:req.Limit]
		lastRaw := rawScans[req.Limit-1]
		nexts := make([]contracts.Value, 0, len(keyIdx))
		for _, ki := range keyIdx {
			v, err2 := sqlToValue(lastRaw[ki])
			if err2 != nil {
				return empty, mapErr(err2)
			}
			nexts = append(nexts, v)
		}
		return contracts.Page{Rows: result, Next: nexts, Done: false}, nil
	}
	return contracts.Page{Rows: collected, Next: nil, Done: true}, nil
}

// valueToSQL returns a sql driver-compatible scalar from a
// contracts.Value. NULL values become typed nil; other kinds are
// encoded per Value.Payload().
func valueToSQL(v contracts.Value) any {
	if v.IsNull() {
		return nil
	}
	switch v.Kind() {
	case contracts.Int:
		n, err := strconv.ParseInt(v.Payload(), 10, 64)
		if err == nil {
			return n
		}
	case contracts.Uint:
		n, err := strconv.ParseUint(v.Payload(), 10, 64)
		if err == nil {
			return n
		}
	case contracts.Float:
		f, err := strconv.ParseFloat(v.Payload(), 64)
		if err == nil {
			return f
		}
	case contracts.Bool:
		switch v.Payload() {
		case "true":
			return true
		case "false":
			return false
		}
	}
	// Fallback: pass payload as string (dates, text, decimal, etc.).
	return v.Payload()
}

// sqlToValue converts a driver-scanned value into contracts.Value.
// All non-nil values are encoded as Text; Kind-specific narrowing must
// be performed by the runner using Describe metadata. The mapping
// intentionally keeps tests and runner expectations simple because
// sample and page outputs feed textual detector rules.
func sqlToValue(v any) (contracts.Value, error) {
	if v == nil {
		return contracts.Value{}, nil
	}
	switch s := v.(type) {
	case nil:
		return contracts.Value{}, nil
	case string:
		return contracts.NewValue(contracts.Text, s)
	case []byte:
		// For safety, preserve textual strings; if binary the runner
		// inspects Kind from Describe.
		return contracts.NewValue(contracts.Text, string(s))
	case bool:
		if s {
			return contracts.NewValue(contracts.Text, "true")
		}
		return contracts.NewValue(contracts.Text, "false")
	case int64:
		return contracts.NewValue(contracts.Text, strconv.FormatInt(s, 10))
	case float64:
		return contracts.NewValue(contracts.Text, strconv.FormatFloat(s, 'f', -1, 64))
	case time.Time:
		return contracts.NewValue(contracts.Text, s.Format(time.RFC3339Nano))
	case int:
		return contracts.NewValue(contracts.Text, strconv.FormatInt(int64(s), 10))
	case int32:
		return contracts.NewValue(contracts.Text, strconv.FormatInt(int64(s), 10))
	case int16:
		return contracts.NewValue(contracts.Text, strconv.FormatInt(int64(s), 10))
	case int8:
		return contracts.NewValue(contracts.Text, strconv.FormatInt(int64(s), 10))
	case uint64:
		return contracts.NewValue(contracts.Text, strconv.FormatUint(s, 10))
	case uint32:
		return contracts.NewValue(contracts.Text, strconv.FormatUint(uint64(s), 10))
	case uint16:
		return contracts.NewValue(contracts.Text, strconv.FormatUint(uint64(s), 10))
	case uint8:
		return contracts.NewValue(contracts.Text, strconv.FormatUint(uint64(s), 10))
	case float32:
		return contracts.NewValue(contracts.Text, strconv.FormatFloat(float64(s), 'f', -1, 64))
	}
	return contracts.Value{}, errors.New("unsupported sql driver type in sqlToValue")
}

// nullSafeEqual returns a dialect-appropriate boolean expression
// comparing a column with a scalar value, including NULL-safe
// semantics. For SQLite it uses `col IS scalar`; for MySQL it uses
// `col <=> scalar`. pos controls the first parameter placeholder
// position and is incremented for every argument consumed.
func nullSafeEqual(d contracts.Dialect, col string, scalar any, pos *int) (string, []any) {
	switch d {
	case contracts.SQLite:
		*pos++
		return fmt.Sprintf("%s IS %s", quoteIdentSQLite(col), placeholder(d, *pos)), []any{scalar}
	case contracts.MySQL:
		*pos++
		return fmt.Sprintf("%s <=> %s", quoteIdentMySQL(col), placeholder(d, *pos)), []any{scalar}
	}
	*pos++
	return fmt.Sprintf("%s = %s", quoteIdent(d, col), placeholder(d, *pos)), []any{scalar}
}

// nullSafeEqualOperator returns a string operator for inlining the
// NULL-safe equality check without re-emitting arguments.
func nullSafeEqualOperator(d contracts.Dialect) string {
	switch d {
	case contracts.SQLite:
		return "IS"
	case contracts.MySQL:
		return "<=>"
	}
	return "="
}
