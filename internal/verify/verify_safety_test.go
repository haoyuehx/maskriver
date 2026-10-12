package verify

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

type boundedReader struct {
	contracts.Reader
	largestPage int
	calls       int
	replay      bool
	cached      contracts.Page
}

func (r *boundedReader) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	r.calls++
	if req.Limit > r.largestPage {
		r.largestPage = req.Limit
	}
	if r.replay && r.calls > 1 {
		return r.cached, nil
	}
	page, err := r.Reader.Page(ctx, req)
	if r.replay && r.calls == 1 {
		r.cached = page
	}
	return page, err
}

func TestSafetyNilContextRejected(t *testing.T) {
	_, err := New().Validate(nil, newFakeReader(), newFakeReader(), contracts.ValidateRequest{
		Plan: peoplePlan(),
	})
	if !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("nil ctx must fail closed: %v", err)
	}
}

func TestSafetyPlannedColumnMissingFromSchema(t *testing.T) {
	ref := peopleTable()
	schema := peopleSchema()
	schema.Columns = append(schema.Columns[:1], schema.Columns[2:]...)
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, nil)
	tgt.addTable(ref, schema, nil)
	report, err := New().Validate(context.Background(), src, tgt, contracts.ValidateRequest{
		Plan: peoplePlan(),
	})
	if err != nil || report.Passed(true) || !hasIssue(report, contracts.SchemaMismatch) {
		t.Fatalf("missing planned column must fail: err=%v issues=%v", err, report.Issues)
	}
}

func TestSafetyPlannedPrimaryKeyChanged(t *testing.T) {
	ref := peopleTable()
	schema := peopleSchema()
	schema.PrimaryKey = []string{"name"}
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, nil)
	tgt.addTable(ref, schema, nil)
	report, err := New().Validate(context.Background(), src, tgt, contracts.ValidateRequest{Plan: peoplePlan()})
	if err != nil || report.Passed(true) || !hasIssue(report, contracts.SchemaMismatch) {
		t.Fatalf("different PK must fail: err=%v issues=%v", err, report.Issues)
	}
}

func TestSafetyMissingColumnCoverageCannotPass(t *testing.T) {
	ref := peopleTable()
	schema := peopleSchema()
	schema.Coverage.Columns = false
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, nil)
	tgt.addTable(ref, schema, nil)
	report, err := New().Validate(context.Background(), src, tgt, contracts.ValidateRequest{Plan: peoplePlan()})
	if err != nil || report.Passed(false) || report.Coverage.Complete ||
		!hasIssue(report, contracts.InsufficientEvidence) {
		t.Fatal("unknown schema column coverage cannot pass")
	}
}

func TestSafetyMissingSelectedRowValueCannotPass(t *testing.T) {
	ref := peopleTable()
	schema := peopleSchema()
	src := newFakeReader()
	tgt := newFakeReader()
	// The key exists, but both rows lack the planned email column.
	row := contracts.Row{"id": mustValue(t, contracts.Int, "1")}
	src.addTable(ref, schema, []contracts.Row{row})
	tgt.addTable(ref, schema, []contracts.Row{row})
	report, err := New().Validate(context.Background(), src, tgt, contracts.ValidateRequest{Plan: peoplePlan()})
	if err != nil || report.Passed(true) || !hasIssue(report, contracts.ConflictingEvidence) {
		t.Fatalf("missing row value must fail, report=%+v err=%v", report, err)
	}
}

func TestSafetyFullScanBoundedPagesAndIssueCounts(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema()
	var original, masked []contracts.Row
	for i := 1; i <= 520; i++ {
		key := mustValue(t, contracts.Int, strconv.Itoa(i))
		original = append(original, contracts.Row{"id": key, "email": textV("synthetic@example.invalid")})
		masked = append(masked, contracts.Row{"id": key, "email": textV("synthetic@example.invalid")})
	}
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, original)
	tgt.addTable(ref, schema, masked)
	bs := &boundedReader{Reader: src}
	bt := &boundedReader{Reader: tgt}
	report, err := New().Validate(context.Background(), bs, bt, contracts.ValidateRequest{
		Plan: peoplePlan(), RowLimit: 0,
	})
	if err != nil || report.Passed(true) || !report.Coverage.Complete || report.Coverage.Rows != 520 {
		t.Fatalf("full scan required, err=%v coverage=%+v", err, report.Coverage)
	}
	if bs.largestPage > pageSize || bt.largestPage > pageSize || bs.calls < 3 || bt.calls < 3 {
		t.Fatal("source and target must use bounded keyset pages")
	}
	if len(report.Issues) != 1 || report.Issues[0].Count != 520 ||
		report.Issues[0].Code != contracts.UnchangedValue {
		t.Fatalf("must aggregate 520 findings into one safe issue: %+v", report.Issues)
	}
}

func TestSafetyStuckCursorCannotPass(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema()
	original := []contracts.Row{}
	masked := []contracts.Row{}
	for i := 1; i <= 4; i++ {
		key := mustValue(t, contracts.Int, strconv.Itoa(i))
		original = append(original, contracts.Row{"id": key, "email": textV("x@example.invalid")})
		masked = append(masked, contracts.Row{"id": key, "email": textV("REDACTED")})
	}
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, original)
	tgt.addTable(ref, schema, masked)
	broken := &boundedReader{Reader: src, replay: true}
	report, err := New().Validate(context.Background(), broken, tgt, contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 2})
	if err != nil || report.Passed(false) || report.Coverage.Complete {
		t.Fatal("truncated input must never pass")
	}
	// Zero budget allows full scan, forcing the replay reader to expose a
	// malformed second page. Both errors and an incomplete report are safe.
	report, err = New().Validate(context.Background(), broken, tgt, contracts.ValidateRequest{Plan: peoplePlan()})
	if err == nil && (report.Passed(false) || report.Coverage.Complete) {
		t.Fatal("repeated cursor must never pass")
	}
}

func TestSafetyPositiveBudgetExactlyAtRowCountPasses(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema()
	var original, masked []contracts.Row
	for i := 1; i <= 10; i++ {
		key := mustValue(t, contracts.Int, strconv.Itoa(i))
		original = append(original, contracts.Row{"id": key, "email": textV("original@example.invalid")})
		masked = append(masked, contracts.Row{"id": key, "email": textV("MASKED")})
	}
	src, tgt := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, original)
	tgt.addTable(ref, schema, masked)
	report, err := New().Validate(context.Background(), src, tgt, contracts.ValidateRequest{
		Plan: peoplePlan(), RowLimit: 10,
	})
	if err != nil || !report.Passed(true) || report.Coverage.Rows != 10 {
		t.Fatalf("exact positive budget should pass, issues=%v err=%v", report.Issues, err)
	}
}
