package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTypedValue(t *testing.T) {
	valid := []struct {
		k Kind
		s string
	}{
		{Null, ""}, {Text, ""}, {Text, "合成测试"}, {Bytes, string([]byte{0, 255})},
		{Int, "-9223372036854775808"}, {Uint, "18446744073709551615"},
		{Float, "1.25"}, {Decimal, "12345678901234567890.1200"}, {Bool, "false"},
		{Date, "2024-02-29"}, {LocalDateTime, "2024-02-29T01:02:03.123456"},
		{Instant, "2024-02-29T01:02:03+08:00"},
	}
	for _, tt := range valid {
		v, err := NewValue(tt.k, tt.s)
		if err != nil || v.Kind() != tt.k {
			t.Fatalf("valid kind %d rejected", tt.k)
		}
		if tt.k != Instant && tt.k != Float && v.Payload() != tt.s {
			t.Fatal("lossy value conversion")
		}
	}
	invalid := []struct {
		k Kind
		s string
	}{
		{Null, "x"}, {Text, string([]byte{255})}, {Int, "01"}, {Int, "9223372036854775808"},
		{Uint, "-1"}, {Float, "NaN"}, {Float, "Inf"}, {Decimal, "1e3"}, {Decimal, "00.1"},
		{Bool, "1"}, {Date, "2025-02-29"}, {Date, "0000-01-01"},
		{LocalDateTime, "2024-01-01T01:02:03Z"}, {Instant, "2024-01-01"}, {Kind(255), "x"},
		{Instant, "2024-01-01T01:02:03.1234567891Z"}, {Instant, "2024-01-01T01:02:03+24:00"},
		{Instant, "0001-01-01T00:00:00+01:00"},
	}
	for _, tt := range invalid {
		if _, err := NewValue(tt.k, tt.s); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid kind %d accepted", tt.k)
		}
	}
	var null Value
	empty, _ := NewValue(Text, "")
	if !null.IsNull() || empty.IsNull() || null == empty {
		t.Fatal("NULL and empty text conflated")
	}
	a, _ := NewValue(Instant, "2024-02-29T01:02:03+08:00")
	b, _ := NewValue(Instant, "2024-02-28T17:02:03Z")
	if a != b {
		t.Fatal("instants not normalized to UTC")
	}
}

func TestValueRedaction(t *testing.T) {
	const sensitive = "synthetic-secret-must-not-appear"
	v, _ := NewValue(Text, sensitive)
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
		if strings.Contains(fmt.Sprintf(format, v), sensitive) {
			t.Fatal("fmt leak")
		}
	}
	data, err := json.Marshal(struct {
		Value Value
		Row   Row
	}{v, Row{"key": v}})
	if err != nil || strings.Contains(string(data), sensitive) {
		t.Fatal("JSON leak")
	}
	if err = json.Unmarshal(data, &v); !errors.Is(err, ErrUnsupported) {
		t.Fatal("redacted JSON must not become executable data")
	}
}

func samplePlan() Plan {
	table := TableRef{"synthetic", "main", "people"}
	return Plan{Revision: APIRevision, ID: "plan", SourceSnapshotID: "snapshot", TargetID: "target", SchemaDigest: "schema", ConfigDigest: "config", ScanComplete: true, Tables: []TablePlan{{Table: table, PrimaryKey: []string{"id"}, Columns: []ColumnPlan{{Column: ColumnRef{table, "email"}, Type: Type{Kind: Text}, Strategy: StrategyRef{"redact", "v1"}, Scope: "email", NormalizationVersion: "typed-lexical-v1", KeyID: "test-key-id"}}}}}
}
func TestPlanShape(t *testing.T) {
	if err := (Plan{}).ValidateShape(); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero plan accepted")
	}
	if err := samplePlan().ValidateShape(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*Plan)
		want   error
	}{
		{"incomplete", func(p *Plan) { p.ScanComplete = false }, ErrIncomplete},
		{"unknown", func(p *Plan) { p.Unresolved = []ColumnRef{{}} }, ErrIncomplete},
		{"no key", func(p *Plan) { p.Tables[0].PrimaryKey = nil }, ErrUnsafe},
		{"sensitive pk", func(p *Plan) { p.Tables[0].Columns[0].Column.Column = "id" }, ErrUnsafe},
		{"duplicate", func(p *Plan) { p.Tables = append(p.Tables, p.Tables[0]) }, ErrInvalid},
		{"strategy version", func(p *Plan) { p.Tables[0].Columns[0].Strategy.Version = "" }, ErrInvalid},
		{"foreign column", func(p *Plan) { p.Tables[0].Columns[0].Column.Table = "other" }, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := samplePlan()
			tt.change(&p)
			if !errors.Is(p.ValidateShape(), tt.want) {
				t.Fatal("unsafe shape accepted")
			}
		})
	}
}
func TestValidationGate(t *testing.T) {
	if (ValidationReport{}).Passed(false) {
		t.Fatal("zero report passes")
	}
	if (ValidationReport{PlanID: "empty", Coverage: Coverage{Complete: true}}).Passed(false) {
		t.Fatal("no checked scope passes")
	}
	r := ValidationReport{PlanID: "plan", Coverage: Coverage{Complete: true, Tables: 1, Columns: 1}}
	if !r.Passed(true) {
		t.Fatal("complete empty table can pass")
	}
	r.Issues = []Issue{{Status: Warning, Code: Unsupported}}
	if r.Passed(true) || !r.Passed(false) {
		t.Fatal("warning strict policy")
	}
	for _, status := range []Status{StatusUnknown, Fail, Error, Status(255)} {
		r.Issues[0].Status = status
		if r.Passed(false) {
			t.Fatal("failure status passes")
		}
	}
	r.Issues = nil
	r.Coverage.Complete = false
	if r.Passed(false) {
		t.Fatal("partial coverage passes")
	}
}
