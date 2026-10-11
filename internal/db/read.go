package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func (r *reader) Count(ctx context.Context, ref contracts.TableRef) (int64, error) {
	if err := r.check(ctx); err != nil {
		return 0, err
	}
	// Refuse cross-dataset and out-of-allowlist schema requests before
	// querying metadata, even if the connection has broader DB grants.
	if err := r.table(ref); err != nil {
		return 0, err
	}
	if err := r.exists(ctx, ref); err != nil {
		return 0, err
	}
	var n int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+r.qualified(ref)).Scan(&n)
	return n, dbError(err)
}

func (r *reader) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	var empty contracts.Sample
	if err := r.check(ctx); err != nil {
		return empty, err
	}
	if req.Limit < 1 || req.Limit > 10000 || req.Column.Column == "" {
		return empty, contracts.ErrInvalid
	}
	schema, err := r.describe(ctx, req.Column.TableRef)
	if err != nil {
		return empty, err
	}
	var col *contracts.Column
	for i := range schema.Columns {
		if schema.Columns[i].Ref.Column == req.Column.Column {
			col = &schema.Columns[i]
			break
		}
	}
	if col == nil {
		return empty, contracts.ErrInvalid
	}
	name := r.quote(req.Column.Column)
	query := "SELECT DISTINCT " + name + " FROM " + r.qualified(req.Column.TableRef) + " WHERE " + name + " IS NOT NULL LIMIT ?"
	rows, err := r.db.QueryContext(ctx, query, req.Limit+1)
	if err != nil {
		return empty, dbError(err)
	}
	vals := make([]contracts.Value, 0, req.Limit+1)
	for rows.Next() {
		var raw any
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var v contracts.Value
		v, err = decode(raw, col.Type)
		if err != nil {
			break
		}
		vals = append(vals, v)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return empty, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return empty, contracts.ErrDatabase
	}
	truncated := len(vals) > req.Limit
	if truncated {
		vals = vals[:req.Limit]
	}
	return contracts.Sample{Values: vals, Basis: contracts.DistinctNonNull, Truncated: truncated}, nil
}

func (r *reader) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	empty := contracts.Page{}
	if err := r.check(ctx); err != nil {
		return empty, err
	}
	if req.Limit < 1 || req.Limit > 10000 {
		return empty, contracts.ErrInvalid
	}
	schema, err := r.describe(ctx, req.Table)
	if err != nil {
		return empty, err
	}
	if len(schema.PrimaryKey) == 0 {
		return empty, contracts.ErrUnsupported
	}
	if len(req.KeyColumns) != len(schema.PrimaryKey) {
		return empty, contracts.ErrInvalid
	}
	colTypes := map[string]contracts.Type{}
	for _, c := range schema.Columns {
		colTypes[c.Ref.Column] = c.Type
	}
	for i, key := range req.KeyColumns {
		if key != schema.PrimaryKey[i] {
			return empty, contracts.ErrInvalid
		}
		if colTypes[key].Nullable {
			return empty, contracts.ErrUnsupported
		}
	}
	if len(req.Columns) == 0 || (len(req.After) != 0 && len(req.After) != len(req.KeyColumns)) {
		return empty, contracts.ErrInvalid
	}
	cols := map[string]bool{}
	for _, col := range req.Columns {
		if col == "" || cols[col] {
			return empty, contracts.ErrInvalid
		}
		if _, ok := colTypes[col]; !ok {
			return empty, contracts.ErrInvalid
		}
		cols[col] = true
	}
	for _, key := range req.KeyColumns {
		if !cols[key] {
			return empty, contracts.ErrInvalid
		}
	}
	for i, v := range req.After {
		if v.IsNull() || v.Kind() != colTypes[req.KeyColumns[i]].Kind {
			return empty, contracts.ErrInvalid
		}
	}
	selected := make([]string, len(req.Columns))
	for i, c := range req.Columns {
		selected[i] = r.quote(c)
	}
	query := "SELECT " + strings.Join(selected, ",") + " FROM " + r.qualified(req.Table)
	args := []any{}
	if len(req.After) > 0 {
		terms := make([]string, 0, len(req.KeyColumns))
		for i := range req.KeyColumns {
			parts := make([]string, 0, i+1)
			for j := 0; j < i; j++ {
				parts = append(parts, r.quote(req.KeyColumns[j])+" = ?")
				args = append(args, encode(req.After[j]))
			}
			parts = append(parts, r.quote(req.KeyColumns[i])+" > ?")
			args = append(args, encode(req.After[i]))
			terms = append(terms, "("+strings.Join(parts, " AND ")+")")
		}
		query += " WHERE " + strings.Join(terms, " OR ")
	}
	keys := make([]string, len(req.KeyColumns))
	for i, k := range req.KeyColumns {
		keys[i] = r.quote(k) + " ASC"
	}
	query += " ORDER BY " + strings.Join(keys, ",") + " LIMIT ?"
	args = append(args, req.Limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return empty, dbError(err)
	}
	result := make([]contracts.Row, 0, req.Limit+1)
	for rows.Next() {
		var row contracts.Row
		row, err = scanRow(rows, req.Columns, colTypes)
		if err != nil {
			break
		}
		result = append(result, row)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return empty, dbErrorOrCategory(err)
	}
	if closeErr != nil {
		return empty, contracts.ErrDatabase
	}
	done := len(result) <= req.Limit
	if !done {
		result = result[:req.Limit]
	}
	page := contracts.Page{Rows: result, Done: done}
	if len(result) > 0 {
		last := result[len(result)-1]
		for _, key := range req.KeyColumns {
			page.Next = append(page.Next, last[key])
		}
	}
	return page, nil
}

func scanRow(rows *sql.Rows, names []string, types map[string]contracts.Type) (contracts.Row, error) {
	raw := make([]any, len(names))
	dest := make([]any, len(names))
	for i := range raw {
		dest[i] = &raw[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, dbError(err)
	}
	out := make(contracts.Row, len(names))
	for i, name := range names {
		v, err := decode(raw[i], types[name])
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, nil
}

func decode(raw any, typ contracts.Type) (contracts.Value, error) {
	if raw == nil {
		return contracts.Value{}, nil
	}
	var s string
	switch v := raw.(type) {
	case []byte:
		s = string(append([]byte(nil), v...))
	case string:
		s = v
	case int64:
		s = strconv.FormatInt(v, 10)
	case uint64:
		s = strconv.FormatUint(v, 10)
	case float64:
		s = strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		if v {
			s = "true"
		} else {
			s = "false"
		}
	case time.Time:
		switch typ.Kind {
		case contracts.Date:
			s = v.Format(time.DateOnly)
		case contracts.LocalDateTime:
			s = v.Format("2006-01-02T15:04:05.999999999")
		case contracts.Instant:
			s = v.UTC().Format(time.RFC3339Nano)
		default:
			return contracts.Value{}, contracts.ErrUnsupported
		}
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if typ.Kind == contracts.LocalDateTime {
		s = strings.Replace(s, " ", "T", 1)
		if dot := strings.IndexByte(s, '.'); dot >= 0 {
			fraction := strings.TrimRight(s[dot+1:], "0")
			if fraction == "" {
				s = s[:dot]
			} else {
				s = s[:dot+1] + fraction
			}
		}
	}
	if typ.Kind == contracts.Bool {
		if s == "1" {
			s = "true"
		} else if s == "0" {
			s = "false"
		}
	}
	if typ.Kind == contracts.Bytes {
		return contracts.NewValue(contracts.Bytes, s)
	}
	v, err := contracts.NewValue(typ.Kind, s)
	if err != nil {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	return v, nil
}

func encode(v contracts.Value) any {
	if v.IsNull() {
		return nil
	}
	if v.Kind() == contracts.Bytes {
		return []byte(v.Payload())
	}
	return v.Payload()
}

func safeType(value contracts.Value, typ contracts.Type) error {
	if value.IsNull() {
		if !typ.Nullable {
			return contracts.ErrInvalid
		}
		return nil
	}
	if value.Kind() != typ.Kind {
		return contracts.ErrInvalid
	}
	return nil
}
