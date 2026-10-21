package db

// Byte-aware, pre-materialisation paging for the M2 streaming path.
//
// contracts.Reader.Page has no byte budget in its frozen signature, so the
// bounded capability is an internal extension implemented by this adapter. The
// caller must treat a reader without it as unsupported and fail closed rather
// than falling back to Page.
//
// PageWithin never asks SQLite for a value that would exceed the per-row
// budget. It reads keys and byte lengths inside one read transaction, rejects
// the page before the value fetch if any row (including the lookahead row) is
// over budget, and only then materialises the rows that passed. The value fetch
// runs in the same transaction, so the lengths and the values describe the same
// SQLite snapshot; this is not a check-then-use pair across independent reads.
//
// TEXT and BLOB lengths are measured with length(CAST(col AS BLOB)); SQLite
// stores both as UTF-8 bytes, so embedded NUL bytes and multi-byte characters
// count correctly. Numeric columns use the exact upper bound of the Go decoder's
// decimal representation, which is never larger than the stored text. NULL
// contributes zero. An unrecognised type refuses the page.

import (
	"context"
	"database/sql"
	"math"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// BoundedReader is the internal byte-aware reader capability. It is deliberately
// narrower than a public contract: only the streaming runner opts in.
type BoundedReader interface {
	contracts.Reader
	PageWithin(context.Context, contracts.PageRequest, int64) (contracts.Page, error)
}

var _ BoundedReader = (*reader)(nil)

// PageWithin reads one keyset page and refuses any row whose projected payload
// exceeds maxRowBytes. It fails closed with ErrUnsafe; it never truncates a row
// and never reports a partial page as complete.
func (r *reader) PageWithin(ctx context.Context, req contracts.PageRequest, maxRowBytes int64) (contracts.Page, error) {
	empty := contracts.Page{}
	if ctx == nil || maxRowBytes <= 0 {
		return empty, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	q, err := r.preparePage(ctx, req)
	if err != nil {
		return empty, err
	}
	lengths, err := rowLengthExprs(q, r)
	if err != nil {
		return empty, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, dbError(err)
	}
	defer func() { _ = tx.Rollback() }()

	// Phase 1: keys plus the complete per-row byte cost. Only integers cross the
	// driver boundary here, never a projected TEXT/BLOB value.
	preflight := "SELECT " + q.keyList(r) + "," + lengths + " FROM " + q.from
	if q.where != "" {
		preflight += " WHERE " + q.where
	}
	preflight += " ORDER BY " + q.order + " LIMIT ?"
	args := append(append([]any{}, q.args...), req.Limit+1)
	rows, err := tx.QueryContext(ctx, preflight, args...)
	if err != nil {
		return empty, dbError(err)
	}
	type candidate struct {
		key  []contracts.Value
		size int64
	}
	candidates := make([]candidate, 0, req.Limit+1)
	var scanErr error
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			scanErr = err
			break
		}
		rawKeys := make([]any, len(req.KeyColumns))
		dest := make([]any, 0, len(rawKeys)+1)
		for i := range rawKeys {
			dest = append(dest, &rawKeys[i])
		}
		var sizeRaw any
		dest = append(dest, &sizeRaw)
		if err := rows.Scan(dest...); err != nil {
			scanErr = err
			break
		}
		size, ok := sizeRaw.(int64)
		if !ok || size < 0 {
			// A SQLite integer overflow degrades the sum to REAL; treat any
			// non-integer cost as an unsafe row instead of an estimate.
			scanErr = contracts.ErrUnsafe
			break
		}
		if size > maxRowBytes {
			scanErr = contracts.ErrUnsafe
			break
		}
		keys := make([]contracts.Value, len(rawKeys))
		for i, raw := range rawKeys {
			v, derr := decode(raw, q.colTypes[req.KeyColumns[i]])
			if derr != nil {
				scanErr = derr
				break
			}
			if v.IsNull() {
				scanErr = contracts.ErrUnsafe
				break
			}
			keys[i] = v
		}
		if scanErr != nil {
			break
		}
		candidates = append(candidates, candidate{key: keys, size: size})
	}
	if scanErr == nil {
		scanErr = rows.Err()
	}
	closeErr := rows.Close()
	if scanErr != nil {
		return empty, dbErrorOrCategory(scanErr)
	}
	if closeErr != nil {
		return empty, contracts.ErrDatabase
	}
	if len(candidates) == 0 {
		if err := tx.Commit(); err != nil {
			return empty, dbError(err)
		}
		return contracts.Page{Rows: []contracts.Row{}, Done: true}, nil
	}

	done := len(candidates) <= req.Limit
	if !done {
		// The lookahead row only proves that another page exists. Its bytes were
		// already checked above; its values are never materialised here.
		candidates = candidates[:req.Limit]
	}
	if len(candidates) == 0 {
		return empty, contracts.ErrIncomplete
	}

	// Phase 2: fetch only the accepted prefix, in the same read transaction.
	// Because the snapshot is stable, this is the same ordered prefix phase 1
	// measured; LIMIT len(candidates) also keeps the driver from materialising
	// the lookahead row.
	query := q.sql(len(candidates))
	valueArgs := append(append([]any{}, q.args...), len(candidates))
	valueRows, err := tx.QueryContext(ctx, query, valueArgs...)
	if err != nil {
		return empty, dbError(err)
	}
	result := make([]contracts.Row, 0, len(candidates))
	for valueRows.Next() {
		if err := ctx.Err(); err != nil {
			scanErr = err
			break
		}
		var row contracts.Row
		row, scanErr = scanRow(valueRows, req.Columns, q.colTypes)
		if scanErr != nil {
			break
		}
		// Defence in depth: the decoder's exact payload can not exceed the
		// preflight estimate, but re-check the real values before returning.
		if rowBytes, ok := decodedRowBytes(row, maxRowBytes); !ok || rowBytes > maxRowBytes {
			scanErr = contracts.ErrUnsafe
			break
		}
		result = append(result, row)
	}
	if scanErr == nil {
		scanErr = valueRows.Err()
	}
	closeErr = valueRows.Close()
	if scanErr != nil {
		return empty, dbErrorOrCategory(scanErr)
	}
	if closeErr != nil {
		return empty, contracts.ErrDatabase
	}
	if len(result) != len(candidates) {
		return empty, contracts.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return empty, dbError(err)
	}
	page := contracts.Page{Rows: result, Done: done}
	last := result[len(result)-1]
	for _, key := range req.KeyColumns {
		page.Next = append(page.Next, last[key])
	}
	return page, nil
}

// keyList renders the ordered key projection used by the preflight query.
func (q *pageQuery) keyList(r *reader) string {
	parts := make([]string, len(q.req.KeyColumns))
	for i, key := range q.req.KeyColumns {
		parts[i] = r.quote(key)
	}
	return joinComma(parts)
}

// rowLengthExprs returns one SQL integer expression that sums the projected
// row's decoded payload bytes. It is exact for TEXT/BLOB/DATE and an upper bound
// for integers, REAL and BOOL, whose Go representation has a fixed maximum.
func rowLengthExprs(q *pageQuery, r *reader) (string, error) {
	parts := make([]string, len(q.req.Columns))
	for i, col := range q.req.Columns {
		quoted := r.quote(col)
		typ := q.colTypes[col]
		switch typ.Kind {
		case contracts.Text, contracts.Bytes, contracts.Date,
			contracts.LocalDateTime, contracts.Instant:
			parts[i] = "COALESCE(length(CAST(" + quoted + " AS BLOB)),0)"
		case contracts.Int:
			// SQLite renders an integer as its exact decimal text, which is the
			// same representation the Go decoder uses.
			parts[i] = "COALESCE(length(CAST(" + quoted + " AS BLOB)),0)"
		case contracts.Uint:
			parts[i] = "CASE WHEN " + quoted + " IS NULL THEN 0 ELSE 20 END"
		case contracts.Float:
			parts[i] = "CASE WHEN " + quoted + " IS NULL THEN 0 ELSE 32 END"
		case contracts.Bool:
			parts[i] = "CASE WHEN " + quoted + " IS NULL THEN 0 ELSE 5 END"
		default:
			// Decimal and unknown kinds have no proven upper bound in SQLite.
			return "", contracts.ErrUnsupported
		}
	}
	return joinPlus(parts), nil
}

// decodedRowBytes sums the actual decoded payload bytes with an overflow guard.
func decodedRowBytes(row contracts.Row, maxRowBytes int64) (int64, bool) {
	var total int64
	for _, value := range row {
		size := int64(len(value.Payload()))
		if size < 0 || total > math.MaxInt64-size {
			return 0, false
		}
		total += size
		if total > maxRowBytes {
			return total, false
		}
	}
	return total, true
}

func joinComma(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ","
		}
		out += part
	}
	return out
}

func joinPlus(parts []string) string {
	out := "0"
	for _, part := range parts {
		out += "+(" + part + ")"
	}
	return out
}
