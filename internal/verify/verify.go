// Package verify checks frozen masking plans against source and target snapshots.
// It never writes to a reader and never includes keys or data in reports.
package verify

import (
	"context"
	"math/big"
	"strconv"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const pageSize = 256

type validator struct{}

var _ contracts.Validator = (*validator)(nil)

// New returns the read-only, bounded-memory validation component.
func New() contracts.Validator { return &validator{} }

// Validate checks every planned table, its schema, row count, both key sets and
// the values of planned sensitive columns. RowLimit=0 checks every row in
// bounded pages; positive RowLimit is a TOTAL budget, not a page size.
func (*validator) Validate(ctx context.Context, source, target contracts.Reader, req contracts.ValidateRequest) (contracts.ValidationReport, error) {
	report := contracts.ValidationReport{PlanID: req.Plan.ID}
	if ctx == nil || source == nil || target == nil || req.RowLimit < 0 {
		return report, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := req.Plan.ValidateShape(); err != nil {
		return report, err
	}
	complete := true
	for _, plan := range req.Plan.Tables {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		srcSchema, err := source.Describe(ctx, plan.Table)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		dstSchema, err := target.Describe(ctx, plan.Table)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		// Coverage.Checks is gated: an adapter that cannot expose CHECK
		// constraints, or whose DDL could not be tokenised unambiguously,
		// reports false, and validation must not claim PASS on it.
		if !srcSchema.Coverage.Columns || !dstSchema.Coverage.Columns ||
			!srcSchema.Coverage.PrimaryKey || !dstSchema.Coverage.PrimaryKey ||
			!srcSchema.Coverage.Indexes || !dstSchema.Coverage.Indexes ||
			!srcSchema.Coverage.Unique || !dstSchema.Coverage.Unique ||
			!srcSchema.Coverage.ForeignKeys || !dstSchema.Coverage.ForeignKeys ||
			!srcSchema.Coverage.Checks || !dstSchema.Coverage.Checks {
			report.Issues = append(report.Issues, issue(contracts.Warning, contracts.InsufficientEvidence, 1))
			complete = false
			continue
		}
		if keyCollationsDiffer(srcSchema, dstSchema, plan.PrimaryKey) ||
			!safeKeyOrder(srcSchema, plan.PrimaryKey) || !safeKeyOrder(dstSchema, plan.PrimaryKey) {
			return report, contracts.ErrUnsupported
		}
		if !schemasMatch(srcSchema, dstSchema, plan) {
			report.Issues = append(report.Issues, issue(contracts.Fail, contracts.SchemaMismatch, 1))
			complete = false
			continue
		}
		srcCount, err := source.Count(ctx, plan.Table)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		dstCount, err := target.Count(ctx, plan.Table)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		if srcCount < 0 || dstCount < 0 {
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		if srcCount != dstCount {
			// Never put raw keys or values in a report.
			report.Issues = append(report.Issues, issue(contracts.Fail, contracts.RowCountMismatch, 1))
		}
		if req.RowLimit > 0 && (srcCount > req.RowLimit || dstCount > req.RowLimit) {
			report.Issues = append(report.Issues, issue(contracts.Warning, contracts.PartialCoverage, 1))
			complete = false
			continue
		}
		cols := plannedColumns(plan)
		src := newCursor(source, plan.Table, cols, plan.PrimaryKey, req.RowLimit)
		dst := newCursor(target, plan.Table, cols, plan.PrimaryKey, req.RowLimit)
		scanned, err := compareRows(ctx, &src, &dst, plan, &report)
		if err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.DatabaseFailure, 1))
			complete = false
			continue
		}
		if src.read != srcCount || dst.read != dstCount {
			report.Issues = append(report.Issues, issue(contracts.Error, contracts.PartialCoverage, 1))
			complete = false
			continue
		}
		report.Coverage.Tables++
		report.Coverage.Columns += int64(len(plan.Columns))
		report.Coverage.Rows += scanned
	}
	report.Coverage.Complete = complete &&
		report.Coverage.Tables == int64(len(req.Plan.Tables))
	return report, nil
}

func issue(status contracts.Status, code contracts.IssueCode, count int64) contracts.Issue {
	return contracts.Issue{Status: status, Code: code, Count: count}
}

// addIssue aggregates by safe category and location; the report never grows
// with row count or records a raw key or sensitive field value.
func addIssue(r *contracts.ValidationReport, next contracts.Issue) {
	for i := range r.Issues {
		prev := &r.Issues[i]
		if prev.Status == next.Status && prev.Code == next.Code && prev.Column == next.Column {
			prev.Count += next.Count
			return
		}
	}
	r.Issues = append(r.Issues, next)
}

func plannedColumns(plan contracts.TablePlan) []string {
	out := make([]string, 0, len(plan.PrimaryKey)+len(plan.Columns))
	out = append(out, plan.PrimaryKey...)
	for _, c := range plan.Columns {
		out = append(out, c.Column.Column)
	}
	return out
}

func sameType(a, b contracts.Type) bool {
	return a.Kind == b.Kind && a.Nullable == b.Nullable &&
		a.Length == b.Length && a.Precision == b.Precision &&
		a.Scale == b.Scale && a.Collation == b.Collation
}

func schemasMatch(a, b contracts.TableSchema, plan contracts.TablePlan) bool {
	if len(a.Columns) != len(b.Columns) || len(a.PrimaryKey) != len(plan.PrimaryKey) ||
		len(b.PrimaryKey) != len(plan.PrimaryKey) {
		return false
	}
	for i, key := range plan.PrimaryKey {
		if a.PrimaryKey[i] != key || b.PrimaryKey[i] != key {
			return false
		}
	}
	src := make(map[string]contracts.Type, len(a.Columns))
	dst := make(map[string]contracts.Type, len(b.Columns))
	for _, c := range a.Columns {
		if c.Ref.TableRef != plan.Table || c.Ref.Column == "" {
			return false
		}
		if _, duplicate := src[c.Ref.Column]; duplicate {
			return false
		}
		src[c.Ref.Column] = c.Type
	}
	for _, c := range b.Columns {
		if c.Ref.TableRef != plan.Table || c.Ref.Column == "" {
			return false
		}
		if _, duplicate := dst[c.Ref.Column]; duplicate {
			return false
		}
		dst[c.Ref.Column] = c.Type
	}
	for name, typ := range src {
		t, ok := dst[name]
		if !ok || !sameType(typ, t) {
			return false
		}
	}
	for _, col := range plan.Columns {
		typ, ok := src[col.Column.Column]
		if !ok || !sameType(typ, col.Type) {
			return false
		}
	}
	return indexesMatch(a.Indexes, b.Indexes) &&
		uniqueGroupsMatch(a.Unique, b.Unique) &&
		foreignKeysMatch(a.ForeignKeys, b.ForeignKeys) &&
		checksMatch(a.Checks, b.Checks)
}

// checksMatch compares CHECK expressions as a multiset after collapsing
// whitespace. Declaration order is not significant.
func checksMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, expr := range a {
		key := normalizeCheck(expr)
		if key == "" {
			return false
		}
		counts[key]++
	}
	for _, expr := range b {
		key := normalizeCheck(expr)
		if counts[key] == 0 {
			return false
		}
		counts[key]--
	}
	return true
}

// normalizeCheck collapses whitespace runs OUTSIDE quoted runs only. Collapsing
// inside a literal would make 'alpha  beta' and 'alpha beta' compare equal, which
// is a false PASS on a semantically different constraint.
func normalizeCheck(expr string) string {
	var b strings.Builder
	b.Grow(len(expr))
	var quote byte
	pendingSpace := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				if i+1 < len(expr) && expr[i+1] == quote {
					b.WriteByte(expr[i+1])
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			pendingSpace = b.Len() > 0
			continue
		case '\'', '"', '`':
			quote = c
		case '[':
			if pendingSpace {
				b.WriteByte(' ')
				pendingSpace = false
			}
			// Bracket identifiers are emitted verbatim up to the closing ']'.
			end := strings.IndexByte(expr[i:], ']')
			if end < 0 {
				b.WriteString(expr[i:])
				return b.String()
			}
			b.WriteString(expr[i : i+end+1])
			i += end
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteByte(c)
	}
	return b.String()
}

// constraintKey encodes ordered string lists unambiguously, so that two
// different constraints can never share a comparison key.
func constraintKey(lists ...[]string) string {
	var b strings.Builder
	for _, list := range lists {
		b.WriteString(strconv.Itoa(len(list)))
		b.WriteByte('|')
		for _, part := range list {
			b.WriteString(strconv.Itoa(len(part)))
			b.WriteByte(':')
			b.WriteString(part)
		}
	}
	return b.String()
}

// indexesMatch compares indexes by name as a multiset. Driver row order is not
// significant, but a duplicate name is counted on both sides so an asymmetric
// substitution cannot slip through. A same-name index must keep its ordered
// column list and its uniqueness flag.
func indexesMatch(a, b []contracts.Index) bool {
	if len(a) != len(b) {
		return false
	}
	collect := func(list []contracts.Index) (map[string]int, bool) {
		out := make(map[string]int, len(list))
		for _, idx := range list {
			if idx.Name == "" || len(idx.Columns) == 0 {
				return nil, false
			}
			out[constraintKey([]string{idx.Name}, boolKey(idx.Unique), idx.Columns)]++
		}
		return out, true
	}
	src, ok := collect(a)
	if !ok {
		return false
	}
	dst, ok := collect(b)
	if !ok {
		return false
	}
	return sameCounts(src, dst)
}

// uniqueGroupsMatch compares unique column groups as a multiset: group order is
// not significant, column order inside a group is (it is index order), and a
// repeated group is counted on both sides rather than rejected.
func uniqueGroupsMatch(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	collect := func(list [][]string) (map[string]int, bool) {
		out := make(map[string]int, len(list))
		for _, group := range list {
			if len(group) == 0 {
				return nil, false
			}
			out[constraintKey(group)]++
		}
		return out, true
	}
	src, ok := collect(a)
	if !ok {
		return false
	}
	dst, ok := collect(b)
	if !ok {
		return false
	}
	return sameCounts(src, dst)
}

// boolKey renders a flag as a stable constraint component.
func boolKey(value bool) []string {
	if value {
		return []string{"1"}
	}
	return []string{"0"}
}

// sameCounts reports whether two multisets are identical.
func sameCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, count := range a {
		if b[key] != count {
			return false
		}
	}
	return true
}

// foreignKeysMatch compares foreign keys as a multiset: declared order is not
// significant, but column pairing order and referential actions are, and a
// repeated descriptor is counted on both sides rather than rejected.
func foreignKeysMatch(a, b []contracts.ForeignKey) bool {
	if len(a) != len(b) {
		return false
	}
	collect := func(list []contracts.ForeignKey) (map[string]int, bool) {
		out := make(map[string]int, len(list))
		for _, fk := range list {
			if len(fk.Columns) == 0 || len(fk.Columns) != len(fk.TargetColumns) ||
				fk.Target.Database == "" || fk.Target.Schema == "" || fk.Target.Table == "" {
				return nil, false
			}
			out[constraintKey(fk.Columns, fk.TargetColumns,
				[]string{fk.Target.Database, fk.Target.Schema, fk.Target.Table},
				[]string{fk.OnUpdate, fk.OnDelete})]++
		}
		return out, true
	}
	src, ok := collect(a)
	if !ok {
		return false
	}
	dst, ok := collect(b)
	if !ok {
		return false
	}
	return sameCounts(src, dst)
}

func keyCollationsDiffer(a, b contracts.TableSchema, keys []string) bool {
	lookup := func(schema contracts.TableSchema, name string) (string, bool) {
		for _, col := range schema.Columns {
			if col.Ref.Column == name {
				return col.Type.Collation, true
			}
		}
		return "", false
	}
	for _, key := range keys {
		left, aOK := lookup(a, key)
		right, bOK := lookup(b, key)
		if !aOK || !bOK || left != right {
			return true
		}
	}
	return false
}

// A non-binary string collation might order keys differently from Go and
// differently across engines. Refuse rather than silently align wrong rows.
func safeKeyOrder(schema contracts.TableSchema, keys []string) bool {
	types := make(map[string]contracts.Type, len(schema.Columns))
	for _, col := range schema.Columns {
		types[col.Ref.Column] = col.Type
	}
	for _, name := range keys {
		t, ok := types[name]
		if !ok || t.Nullable {
			return false
		}
		switch t.Kind {
		case contracts.Int, contracts.Uint, contracts.Bool, contracts.Date:
		case contracts.Text:
			if t.Collation != "" && !strings.EqualFold(t.Collation, "binary") {
				// Includes MySQL _bin collations, whose PAD SPACE semantics
				// are not faithfully implemented by strings.Compare.
				return false
			}
		default:
			return false
		}
	}
	return true
}

type cursor struct {
	reader contracts.Reader
	table  contracts.TableRef
	cols   []string
	keys   []string
	limit  int
	page   []contracts.Row
	pos    int
	done   bool
	after  []contracts.Value
	prev   []contracts.Value
	read   int64
}

func newCursor(r contracts.Reader, table contracts.TableRef, cols, keys []string, max int64) cursor {
	limit := pageSize
	if max > 0 && max < int64(limit) {
		limit = int(max)
	}
	return cursor{reader: r, table: table, cols: cols, keys: keys, limit: limit}
}

func (c *cursor) next(ctx context.Context) (contracts.Row, bool, error) {
	for c.pos >= len(c.page) {
		if c.done {
			return nil, false, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		page, err := c.reader.Page(ctx, contracts.PageRequest{
			Table: c.table, Columns: c.cols, KeyColumns: c.keys,
			After: c.after, Limit: c.limit,
		})
		if err != nil {
			return nil, false, err
		}
		if len(page.Rows) == 0 && !page.Done || len(page.Rows) > c.limit ||
			!page.Done && len(page.Next) != len(c.keys) {
			return nil, false, contracts.ErrIncomplete
		}
		c.page, c.pos, c.done = page.Rows, 0, page.Done
		if !page.Done {
			last, ok := rowKey(page.Rows[len(page.Rows)-1], c.keys)
			if !ok {
				return nil, false, contracts.ErrIncomplete
			}
			cmp, ok := compareKeys(last, page.Next)
			if !ok || cmp != 0 {
				return nil, false, contracts.ErrIncomplete
			}
			if len(c.after) > 0 {
				cmp, ok := compareKeys(c.after, page.Next)
				if !ok || cmp >= 0 {
					return nil, false, contracts.ErrIncomplete
				}
			}
			c.after = append([]contracts.Value(nil), page.Next...)
		}
	}
	row := c.page[c.pos]
	c.pos++
	key, ok := rowKey(row, c.keys)
	if !ok {
		return nil, false, contracts.ErrIncomplete
	}
	if len(c.prev) > 0 {
		cmp, comparable := compareKeys(c.prev, key)
		if !comparable || cmp >= 0 {
			return nil, false, contracts.ErrIncomplete
		}
	}
	c.prev = key
	c.read++
	return row, true, nil
}

func rowKey(row contracts.Row, keys []string) ([]contracts.Value, bool) {
	out := make([]contracts.Value, len(keys))
	for i, name := range keys {
		v, ok := row[name]
		if !ok || v.IsNull() {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// compareKeys uses typed numeric comparison, never lexical numeric ordering.
// Non-binary text collation was already rejected by safeKeyOrder.
func compareKeys(a, b []contracts.Value) (int, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	for i, av := range a {
		bv := b[i]
		if av.Kind() != bv.Kind() || av.IsNull() || bv.IsNull() {
			return 0, false
		}
		var cmp int
		switch av.Kind() {
		case contracts.Int, contracts.Uint:
			var left, right big.Int
			if _, ok := left.SetString(av.Payload(), 10); !ok {
				return 0, false
			}
			if _, ok := right.SetString(bv.Payload(), 10); !ok {
				return 0, false
			}
			cmp = left.Cmp(&right)
		case contracts.Text, contracts.Date:
			cmp = strings.Compare(av.Payload(), bv.Payload())
		case contracts.Bool:
			cmp = strings.Compare(av.Payload(), bv.Payload())
		default:
			return 0, false
		}
		if cmp != 0 {
			return cmp, true
		}
	}
	return 0, true
}

func compareRows(ctx context.Context, src, dst *cursor, plan contracts.TablePlan, report *contracts.ValidationReport) (int64, error) {
	var verified int64
	left, hasLeft, err := src.next(ctx)
	if err != nil {
		return 0, err
	}
	right, hasRight, err := dst.next(ctx)
	if err != nil {
		return 0, err
	}
	// Streaming merge join: at most one page per reader is retained.
	for hasLeft || hasRight {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		switch {
		case !hasLeft:
			addIssue(report, issue(contracts.Fail, contracts.KeyMismatch, 1))
			right, hasRight, err = dst.next(ctx)
		case !hasRight:
			addIssue(report, issue(contracts.Fail, contracts.KeyMismatch, 1))
			left, hasLeft, err = src.next(ctx)
		default:
			cmp, ok := compareKeys(src.prev, dst.prev)
			if !ok {
				return 0, contracts.ErrIncomplete
			}
			switch {
			case cmp < 0:
				addIssue(report, issue(contracts.Fail, contracts.KeyMismatch, 1))
				left, hasLeft, err = src.next(ctx)
			case cmp > 0:
				addIssue(report, issue(contracts.Fail, contracts.KeyMismatch, 1))
				right, hasRight, err = dst.next(ctx)
			default:
				for _, col := range plan.Columns {
					a, aOK := left[col.Column.Column]
					b, bOK := right[col.Column.Column]
					if !aOK || !bOK || a.Kind() != b.Kind() {
						addIssue(report, contracts.Issue{
							Status: contracts.Fail, Code: contracts.ConflictingEvidence,
							Column: col.Column, Count: 1,
						})
					} else if a.IsNull() != b.IsNull() {
						addIssue(report, contracts.Issue{
							Status: contracts.Fail, Code: contracts.ConflictingEvidence,
							Column: col.Column, Count: 1,
						})
					} else if !a.IsNull() && a == b {
						addIssue(report, contracts.Issue{
							Status: contracts.Fail, Code: contracts.UnchangedValue,
							Column: col.Column, Count: 1,
						})
					}
				}
				verified++
				left, hasLeft, err = src.next(ctx)
				if err == nil {
					right, hasRight, err = dst.next(ctx)
				}
			}
		}
		if err != nil {
			return 0, err
		}
	}
	return verified, nil
}
