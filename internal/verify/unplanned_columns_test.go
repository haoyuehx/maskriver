package verify

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// projectionReader models adapters that return only explicitly requested columns.
// This ensures strict verification actually requests the unplanned columns.
type projectionReader struct {
	*fakeReader
	requested bool
}

func (r *projectionReader) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	for _, name := range req.Columns {
		if name == "name" {
			r.requested = true
		}
	}
	page, err := r.fakeReader.Page(ctx, req)
	if err != nil {
		return page, err
	}
	for i, row := range page.Rows {
		projected := make(contracts.Row, len(req.Columns))
		for _, name := range req.Columns {
			if value, ok := row[name]; ok {
				projected[name] = value
			}
		}
		page.Rows[i] = projected
	}
	return page, nil
}

func TestStrictUnplannedColumns(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema() // no UNIQUE constraints
	key := mustValue(t, contracts.Int, "1")
	original := contracts.Row{"id": key, "email": textV("original-secret"), "name": textV("preserved")}
	masked := contracts.Row{"id": key, "email": textV("redacted"), "name": textV("preserved")}
	for _, tc := range []struct {
		name     string
		src, dst contracts.Row
		pass     bool
	}{
		{"unchanged", original, masked, true},
		{"changed", original, contracts.Row{"id": key, "email": textV("redacted"), "name": textV("tampered")}, false},
		{"null-equal", contracts.Row{"id": key, "email": textV("original-secret"), "name": {}}, contracts.Row{"id": key, "email": textV("redacted"), "name": {}}, true},
		{"null-changed", contracts.Row{"id": key, "email": textV("original-secret"), "name": {}}, masked, false},
		{"kind-changed", original, contracts.Row{"id": key, "email": textV("redacted"), "name": mustValue(t, contracts.Int, "7")}, false},
		{"missing-target-column", original, contracts.Row{"id": key, "email": textV("redacted")}, false},
		{"missing-both-columns", contracts.Row{"id": key, "email": textV("original-secret")}, contracts.Row{"id": key, "email": textV("redacted")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, dst := newFakeReader(), newFakeReader()
			src.addTable(ref, schema, []contracts.Row{tc.src})
			dst.addTable(ref, schema, []contracts.Row{tc.dst})
			ps, pd := &projectionReader{fakeReader: src}, &projectionReader{fakeReader: dst}
			rep, err := New().Validate(context.Background(), ps, pd, contracts.ValidateRequest{Plan: peoplePlan(), Strict: true})
			if err != nil || rep.Passed(true) != tc.pass {
				t.Fatalf("strict pass=%v, expected=%v, err=%v, issues=%v", rep.Passed(true), tc.pass, err, rep.Issues)
			}
			if !ps.requested || !pd.requested {
				t.Fatal("strict reader did not request the unplanned column")
			}
			if !tc.pass && !hasIssue(rep, contracts.ConflictingEvidence) {
				t.Fatalf("expected safe conflict evidence: %v", rep.Issues)
			}
			if strings.Contains(strings.ToLower(rep.PlanID), "secret") {
				t.Fatal("report leaked field value")
			}
			for _, issue := range rep.Issues {
				if strings.Contains(strings.ToLower(issue.Column.Column), "secret") {
					t.Fatal("report leaked field value")
				}
			}
			encoded, err := json.Marshal(rep)
			if err != nil || strings.Contains(string(encoded), "original-secret") || strings.Contains(string(encoded), "tampered") || strings.Contains(string(encoded), "preserved") {
				t.Fatalf("report JSON includes raw data or cannot marshal: %v", err)
			}
		})
	}
}

func TestStrictUnplannedColumnsAcrossPages(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema()
	var originals, masked []contracts.Row
	for i := 1; i <= pageSize+2; i++ {
		key := mustValue(t, contracts.Int, strconv.Itoa(i))
		originals = append(originals, contracts.Row{"id": key, "email": textV("original"), "name": textV("same")})
		name := "same"
		if i == pageSize+1 {
			name = "changed"
		}
		masked = append(masked, contracts.Row{"id": key, "email": textV("masked"), "name": textV(name)})
	}
	src, dst := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, originals)
	dst.addTable(ref, schema, masked)
	bs, bt := &boundedReader{Reader: src}, &boundedReader{Reader: dst}
	rep, err := New().Validate(context.Background(), bs, bt, contracts.ValidateRequest{Plan: peoplePlan(), Strict: true})
	if err != nil || rep.Passed(true) || !hasIssue(rep, contracts.ConflictingEvidence) ||
		rep.Coverage.Rows != int64(len(originals)) || bs.calls < 2 || bt.calls < 2 ||
		bs.largestPage > pageSize || bt.largestPage > pageSize {
		t.Fatalf("strict pagination mismatch: report=%+v err=%v", rep, err)
	}
}

func TestStrictUnplannedColumnReadFailureIsNotPass(t *testing.T) {
	src, dst, plan := strictFixture(t)
	dst.pageErr[peopleTable()] = errors.New("synthetic read failure")
	rep, err := New().Validate(context.Background(), src, dst, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil || rep.Passed(true) || rep.Coverage.Complete || !hasIssue(rep, contracts.DatabaseFailure) {
		t.Fatalf("read failure must fail closed: %+v, err=%v", rep, err)
	}
}

func TestStrictUnplannedColumnSchemaMismatch(t *testing.T) {
	ref, schema := peopleTable(), peopleSchema()
	other := schema
	other.Columns = append([]contracts.Column(nil), schema.Columns[:2]...)
	src, dst := newFakeReader(), newFakeReader()
	src.addTable(ref, schema, nil)
	dst.addTable(ref, other, nil)
	rep, err := New().Validate(context.Background(), src, dst, contracts.ValidateRequest{Plan: peoplePlan(), Strict: true})
	if err != nil || rep.Passed(true) || !hasIssue(rep, contracts.SchemaMismatch) {
		t.Fatalf("missing schema column must fail: %+v, err=%v", rep, err)
	}
}
