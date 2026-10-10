package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func testColumn() contracts.Column {
	return contracts.Column{
		Ref:  contracts.ColumnRef{TableRef: contracts.TableRef{Database: "d", Schema: "main", Table: "t"}, Column: "c"},
		Type: contracts.Type{Kind: contracts.Text},
	}
}

func testRequest(values []contracts.Value) contracts.DetectionRequest {
	return contracts.DetectionRequest{
		Column:    testColumn(),
		Sample:    contracts.Sample{Values: values, Basis: contracts.DistinctNonNull},
		DateOrder: "MDY",
	}
}

func textValues(t *testing.T, values ...string) []contracts.Value {
	t.Helper()
	out := make([]contracts.Value, len(values))
	for i, s := range values {
		v, err := contracts.NewValue(contracts.Text, s)
		if err != nil {
			t.Fatalf("NewValue(Text) case %d = %v", i, err)
		}
		out[i] = v
	}
	return out
}

func detectOK(t *testing.T, req contracts.DetectionRequest) contracts.Decision {
	t.Helper()
	r, err := New().Detect(context.Background(), req)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	return r
}

func evidenceByRule(t *testing.T, r contracts.Decision, id string) contracts.Evidence {
	t.Helper()
	for _, e := range r.Evidence {
		if e.RuleID == id {
			return e
		}
	}
	t.Fatalf("no evidence for rule %q in %#v", id, r.Evidence)
	return contracts.Evidence{}
}

func hasCode(codes []contracts.IssueCode, want contracts.IssueCode) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

func emails(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("user%02d@example.com", i)
	}
	return out
}

func cnMobiles(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("138%08d", i)
	}
	return out
}

func fillers(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("filler-%02d", i)
	}
	return out
}

func TestNewReturnsUsableDetector(t *testing.T) {
	r := detectOK(t, testRequest(textValues(t, emails(20)...)))
	if r.Sensitivity != contracts.Sensitive || r.Source != contracts.Rule || r.RuleID != "email" {
		t.Fatalf("generic email column = %#v", r)
	}
}

func TestDetectEmailThresholdBoundary(t *testing.T) {
	cases := []struct {
		name     string
		valid    int
		want     contracts.Sensitivity
		eligible bool
		reason   contracts.IssueCode
	}{
		{"19of20", 19, contracts.Sensitive, true, ""},
		{"18of20_exact_090", 18, contracts.Sensitive, true, ""},
		{"17of20_below_090", 17, contracts.Unknown, false, contracts.InsufficientEvidence},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := textValues(t, append(emails(tc.valid), fillers(20-tc.valid)...)...)
			r := detectOK(t, testRequest(values))
			ev := evidenceByRule(t, r, "email")
			if ev.Hits != tc.valid || ev.Total != 20 || ev.Threshold != .9 || ev.Eligible != tc.eligible {
				t.Fatalf("email evidence = %#v", ev)
			}
			if r.Sensitivity != tc.want {
				t.Fatalf("sensitivity = %v, want %v", r.Sensitivity, tc.want)
			}
			if tc.reason == "" {
				if len(ev.Reasons) != 0 {
					t.Fatalf("unexpected reasons %v", ev.Reasons)
				}
			} else if !hasCode(ev.Reasons, tc.reason) {
				t.Fatalf("reasons %v missing %v", ev.Reasons, tc.reason)
			}
		})
	}
}

func TestDetectMinSamplesBoundary(t *testing.T) {
	t.Run("19 distinct below default MinSamples", func(t *testing.T) {
		r := detectOK(t, testRequest(textValues(t, emails(19)...)))
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("want Unknown below MinSamples: %#v", r)
		}
		ev := evidenceByRule(t, r, "email")
		if ev.Hits != 19 || ev.Total != 19 || ev.Eligible || !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
			t.Fatalf("email evidence = %#v", ev)
		}
	})
	t.Run("20 distinct meets default MinSamples", func(t *testing.T) {
		r := detectOK(t, testRequest(textValues(t, emails(20)...)))
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("want Sensitive at MinSamples: %#v", r)
		}
	})
	t.Run("custom MinSamples accepts 19", func(t *testing.T) {
		req := testRequest(textValues(t, emails(19)...))
		req.MinSamples = 19
		if r := detectOK(t, req); r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("custom MinSamples: %#v", r)
		}
	})
	t.Run("custom MinRatio accepts 17 of 20", func(t *testing.T) {
		req := testRequest(textValues(t, append(emails(17), fillers(3)...)...))
		req.MinRatio = 0.8
		if r := detectOK(t, req); r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("custom MinRatio: %#v", r)
		}
	})
}

func TestDetectDistinctNonBlankDenominator(t *testing.T) {
	valid := emails(19)
	raw := append([]string{}, valid...)
	raw = append(raw, valid[0], "not-an-email", "   ", "")
	values := textValues(t, raw...)
	values = append(values, contracts.Value{}) // SQL NULL
	r := detectOK(t, testRequest(values))
	ev := evidenceByRule(t, r, "email")
	if ev.Hits != 19 || ev.Total != 20 {
		t.Fatalf("email evidence = %#v, want 19/20 after dedup and blank/NULL removal", ev)
	}
	if r.Sensitivity != contracts.Sensitive {
		t.Fatalf("sensitivity = %v, want Sensitive", r.Sensitivity)
	}
}

func TestDetectRulePositives(t *testing.T) {
	cases := map[string]string{
		"email":      "first.last+tag@sub.example.co.uk",
		"url":        "https://www.example.cn/path?q=1#top",
		"ip_address": "192.168.10.20",
		"uuid":       "550e8400-e29b-41d4-a716-446655440000",
		"cn_date":    "2023年1月2日",
	}
	for ruleID, value := range cases {
		t.Run(ruleID, func(t *testing.T) {
			req := testRequest(textValues(t, value))
			req.MinSamples = 1
			r := detectOK(t, req)
			if r.Sensitivity != contracts.Sensitive || r.Source != contracts.Rule || r.RuleID != ruleID {
				t.Fatalf("rule %s positive rejected: %#v", ruleID, r)
			}
		})
	}
}

func TestDetectRuleNegatives(t *testing.T) {
	cases := map[string][]string{
		"email": {
			"a..b@example.com", ".a@example.com", "a.@example.com", "a@example..com",
			"a@.example.com", "a@example.com.", "a@-example.com", "a@example",
			"a@exa_mple.com", "a@example.c", "a@example.123", "a b@example.com",
			"a@@b.com", `"quoted"@example.com`,
		},
		"url": {
			"https://?", "http://example.com:abc", "http://example.com:99999",
			"http://example.com:", "http://-bad.com", "http://.com", "http://exa mple.com",
			"http://foo", "ftp://example.com", "http://example.com.", "http://example..com",
			"example.com",
		},
		"ip_address": {"256.1.1.1", "1.2.3", "1.2.3.4.5", "192.168.000.001", "2001:db8::1%eth0", "not-an-ip"},
		"uuid": {
			"550e8400-e29b-01d4-a716-446655440000", "550e8400-e29b-61d4-a716-446655440000",
			"550e8400-e29b-41d4-7716-446655440000", "550e8400-e29b-41d4-c716-446655440000",
			"00000000-0000-0000-0000-000000000000", "550e8400e29b41d4a716446655440000",
		},
		"cn_date":   {"0000-01-01", "2023-02-29", "2023-13-01", "2023-1-1", "01/02/2023", "2023年2月29日", "2023年13月1日", "not-a-date"},
		"cn_mobile": {"12345", "23800138000", "1380013800", "138001380000", "138-0013-8000", "13800138000x", "abc"},
	}
	for ruleID, values := range cases {
		for i, value := range values {
			t.Run(fmt.Sprintf("%s/%d", ruleID, i), func(t *testing.T) {
				req := testRequest(textValues(t, value))
				req.MinSamples = 1
				r := detectOK(t, req)
				if r.Sensitivity != contracts.Unknown {
					t.Fatalf("rule %s negative case %d matched %q; want Unknown", ruleID, i, r.RuleID)
				}
				ev := evidenceByRule(t, r, ruleID)
				if ev.Hits != 0 || ev.Eligible {
					t.Fatalf("evidence for %s case %d = %#v", ruleID, i, ev)
				}
				if !hasCode(ev.Reasons, contracts.Unsupported) {
					t.Fatalf("evidence reasons for %s case %d = %v, want Unsupported", ruleID, i, ev.Reasons)
				}
			})
		}
	}
}

func TestDetectIPAddressLiterals(t *testing.T) {
	valid := []string{
		"192.168.10.20",
		"0.0.0.0",
		"255.255.255.255",
		"2001:db8::1",
		"2001:0db8:0000:0000:0000:0000:0000:0001",
		"::ffff:192.168.0.1",
		"::1",
	}
	invalid := []string{
		"256.1.1.1",
		"1.2.3",
		"1.2.3.4.5",
		"192.168.000.001",
		"2001:db8::1%eth0",
		"fe80::1%25eth0",
		"not-an-ip",
	}
	for i, value := range valid {
		req := testRequest(textValues(t, value))
		req.MinSamples = 1
		if r := detectOK(t, req); r.RuleID != "ip_address" {
			t.Fatalf("valid ip case %d not detected: %#v", i, r)
		}
	}
	for i, value := range invalid {
		req := testRequest(textValues(t, value))
		req.MinSamples = 1
		if r := detectOK(t, req); r.Sensitivity != contracts.Unknown {
			t.Fatalf("invalid ip case %d accepted as %q", i, r.RuleID)
		}
	}
}

func TestDetectUUIDVersionsAndVariants(t *testing.T) {
	valid := []string{
		"550e8400-e29b-11d4-a716-446655440000", // version 1
		"550e8400-e29b-21d4-a716-446655440000", // version 2
		"550e8400-e29b-31d4-a716-446655440000", // version 3
		"550e8400-e29b-41d4-a716-446655440000", // version 4, variant a
		"550e8400-e29b-51d4-8716-446655440000", // version 5, variant 8
		"550e8400-e29b-41d4-9716-446655440000", // variant 9
		"550E8400-E29B-41D4-B716-446655440000", // uppercase, variant b
	}
	invalid := []string{
		"550e8400-e29b-01d4-a716-446655440000", // version 0
		"550e8400-e29b-61d4-a716-446655440000", // version 6
		"550e8400-e29b-41d4-7716-446655440000", // variant 7
		"550e8400-e29b-41d4-c716-446655440000", // variant c
		"00000000-0000-0000-0000-000000000000", // nil UUID
	}
	for i, value := range valid {
		req := testRequest(textValues(t, value))
		req.MinSamples = 1
		if r := detectOK(t, req); r.RuleID != "uuid" || r.Sensitivity != contracts.Sensitive {
			t.Fatalf("valid uuid case %d not accepted: %#v", i, r)
		}
	}
	for i, value := range invalid {
		req := testRequest(textValues(t, value))
		req.MinSamples = 1
		if r := detectOK(t, req); r.Sensitivity != contracts.Unknown {
			t.Fatalf("invalid uuid case %d accepted as %q", i, r.RuleID)
		}
	}
}

func TestDetectCNDateFormatsAndTypedDate(t *testing.T) {
	valid := []string{"2023-01-02", "2023/01/02", "2023年1月2日", "2024-02-29", "1999/12/31", "2000年10月1日"}
	for i, value := range valid {
		req := testRequest(textValues(t, value))
		req.MinSamples = 1
		if r := detectOK(t, req); r.RuleID != "cn_date" {
			t.Fatalf("cn_date positive case %d not detected: %#v", i, r)
		}
	}
	if cnDateMatch(textValues(t, "0000-01-01")[0], "MDY") {
		t.Fatal("cnDateMatch accepted year 0000")
	}

	typed, err := contracts.NewValue(contracts.Date, "1999-12-31")
	if err != nil {
		t.Fatalf("NewValue(Date) = %v", err)
	}
	req := testRequest([]contracts.Value{typed})
	req.Column.Type.Kind = contracts.Date
	req.MinSamples = 1
	if r := detectOK(t, req); r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_date" {
		t.Fatalf("typed Date = %#v", r)
	}

	for _, order := range []string{"MDY", "DMY"} {
		req := testRequest(textValues(t, "2023-01-02"))
		req.MinSamples = 1
		req.DateOrder = order
		if r := detectOK(t, req); r.RuleID != "cn_date" {
			t.Fatalf("date order %s = %#v", order, r)
		}
	}
}

func TestDetectCNMobileRequiresContext(t *testing.T) {
	t.Run("generic column is not enough", func(t *testing.T) {
		r := detectOK(t, testRequest(textValues(t, cnMobiles(20)...)))
		if r.Sensitivity != contracts.Unknown || r.RuleID != "" {
			t.Fatalf("want Unknown without context: %#v", r)
		}
		ev := evidenceByRule(t, r, "cn_mobile")
		if ev.Hits != 20 || ev.Eligible || !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
			t.Fatalf("cn_mobile evidence = %#v", ev)
		}
	})
	t.Run("order_number column is not enough", func(t *testing.T) {
		req := testRequest(textValues(t, cnMobiles(20)...))
		req.Column.Ref.Column = "order_number"
		if r := detectOK(t, req); r.Sensitivity != contracts.Unknown {
			t.Fatalf("want Unknown for order_number: %#v", r)
		}
	})
	t.Run("phone field is sensitive", func(t *testing.T) {
		req := testRequest(textValues(t, cnMobiles(20)...))
		req.Column.Ref.Column = "phone"
		if r := detectOK(t, req); r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
			t.Fatalf("phone field = %#v", r)
		}
	})
	t.Run("Chinese phone column is sensitive", func(t *testing.T) {
		req := testRequest(textValues(t, cnMobiles(20)...))
		req.Column.Ref.Column = "联系电话"
		if r := detectOK(t, req); r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
			t.Fatalf("Chinese phone field = %#v", r)
		}
	})
	t.Run("phone field with emails conflicts", func(t *testing.T) {
		req := testRequest(textValues(t, emails(20)...))
		req.Column.Ref.Column = "phone"
		r := detectOK(t, req)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("want Unknown conflict: %#v", r)
		}
		if !hasCode(evidenceByRule(t, r, "cn_mobile").Reasons, contracts.ConflictingEvidence) {
			t.Fatal("cn_mobile evidence missing conflicting_evidence")
		}
	})
	t.Run("userEmail field with mobile values conflicts", func(t *testing.T) {
		req := testRequest(textValues(t, cnMobiles(20)...))
		req.Column.Ref.Column = "userEmail"
		r := detectOK(t, req)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("want Unknown conflict: %#v", r)
		}
		if !hasCode(evidenceByRule(t, r, "cn_mobile").Reasons, contracts.ConflictingEvidence) {
			t.Fatal("cn_mobile evidence missing conflicting_evidence")
		}
	})
}

func TestDetectFieldContextConflict(t *testing.T) {
	t.Run("phone column with emails", func(t *testing.T) {
		req := testRequest(textValues(t, emails(20)...))
		req.Column.Ref.Column = "phone_number"
		r := detectOK(t, req)
		if r.Sensitivity != contracts.Unknown || r.RuleID != "" {
			t.Fatalf("want Unknown, got %#v", r)
		}
		emailEv := evidenceByRule(t, r, "email")
		if !emailEv.Eligible || emailEv.Hits != 20 {
			t.Fatalf("email evidence = %#v", emailEv)
		}
		if !hasCode(emailEv.Reasons, contracts.ConflictingEvidence) {
			t.Fatalf("email reasons = %v, want conflicting_evidence", emailEv.Reasons)
		}
	})

	t.Run("generic column is decided by format", func(t *testing.T) {
		r := detectOK(t, testRequest(textValues(t, emails(20)...)))
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("got %#v", r)
		}
		if hasCode(evidenceByRule(t, r, "email").Reasons, contracts.ConflictingEvidence) {
			t.Fatal("unexpected conflicting_evidence on generic column")
		}
	})

	t.Run("matching context agrees", func(t *testing.T) {
		req := testRequest(textValues(t, emails(20)...))
		req.Column.Ref.Column = "email_address"
		r := detectOK(t, req)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("got %#v", r)
		}
	})
}

func TestDetectPriorityOrder(t *testing.T) {
	col := testColumn()
	override := &contracts.Decision{Column: col.Ref, Type: col.Type, Sensitivity: contracts.NotSensitive, Source: contracts.Manual}
	reviewed := &contracts.Decision{Column: col.Ref, Type: col.Type, Sensitivity: contracts.Sensitive, Source: contracts.ReviewedHistory}

	req := testRequest(textValues(t, emails(20)...))
	req.Override, req.Reviewed, req.Skip = override, reviewed, true
	r := detectOK(t, req)
	if r.Source != contracts.Manual || r.Sensitivity != contracts.NotSensitive || len(r.Evidence) != 0 {
		t.Fatalf("override priority = %#v", r)
	}

	req.Override = nil
	if r = detectOK(t, req); r.Source != contracts.ReviewedHistory || r.Sensitivity != contracts.Sensitive {
		t.Fatalf("reviewed priority = %#v", r)
	}

	req.Reviewed = nil
	r = detectOK(t, req)
	if r.Source != contracts.Skip || r.Sensitivity != contracts.Unknown || r.RuleID != "" || len(r.Evidence) != 0 {
		t.Fatalf("skip priority (must not imply safe) = %#v", r)
	}

	req.Skip = false
	if r = detectOK(t, req); r.Source != contracts.Rule || r.RuleID != "email" || r.Sensitivity != contracts.Sensitive {
		t.Fatalf("rule priority = %#v", r)
	}
}

func TestDetectForcedDecisionClearsEvidence(t *testing.T) {
	col := testColumn()
	override := &contracts.Decision{
		Column: col.Ref, Type: col.Type, Sensitivity: contracts.NotSensitive, Source: contracts.Manual,
		Evidence: []contracts.Evidence{{RuleID: "unchecked-claim", Hits: 999, Total: 1000}},
	}
	req := testRequest(textValues(t, "not-an-email"))
	req.Override = override
	r := detectOK(t, req)
	if len(r.Evidence) != 0 {
		t.Fatalf("forced decision echoed caller evidence: %#v", r.Evidence)
	}
	if len(override.Evidence) != 1 {
		t.Fatalf("input override was mutated: %#v", override.Evidence)
	}
}

func TestDetectRejectsMismatchedOverrideReviewed(t *testing.T) {
	col := testColumn()
	other := col
	other.Ref.Column = "other"
	wrongType := col
	wrongType.Type.Kind = contracts.Int

	cases := []struct {
		name string
		set  func(*contracts.DetectionRequest)
	}{
		{"override location", func(r *contracts.DetectionRequest) {
			r.Override = &contracts.Decision{Column: other.Ref, Type: col.Type, Sensitivity: contracts.Sensitive, Source: contracts.Manual}
		}},
		{"override type", func(r *contracts.DetectionRequest) {
			r.Override = &contracts.Decision{Column: col.Ref, Type: wrongType.Type, Sensitivity: contracts.Sensitive, Source: contracts.Manual}
		}},
		{"override source", func(r *contracts.DetectionRequest) {
			r.Override = &contracts.Decision{Column: col.Ref, Type: col.Type, Sensitivity: contracts.Sensitive, Source: contracts.Rule}
		}},
		{"override unknown sensitivity", func(r *contracts.DetectionRequest) {
			r.Override = &contracts.Decision{Column: col.Ref, Type: col.Type, Source: contracts.Manual}
		}},
		{"reviewed location", func(r *contracts.DetectionRequest) {
			r.Reviewed = &contracts.Decision{Column: other.Ref, Type: col.Type, Sensitivity: contracts.Sensitive, Source: contracts.ReviewedHistory}
		}},
		{"reviewed source", func(r *contracts.DetectionRequest) {
			r.Reviewed = &contracts.Decision{Column: col.Ref, Type: col.Type, Sensitivity: contracts.Sensitive, Source: contracts.Manual}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testRequest(textValues(t, "not-an-email"))
			tc.set(&req)
			if _, err := New().Detect(context.Background(), req); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestDetectRejectsInvalidRequest(t *testing.T) {
	intValue, err := contracts.NewValue(contracts.Int, "1")
	if err != nil {
		t.Fatalf("NewValue(Int) = %v", err)
	}
	cases := []struct {
		name string
		set  func(*contracts.DetectionRequest)
	}{
		{"empty column", func(r *contracts.DetectionRequest) { r.Column.Ref.Column = "" }},
		{"invalid kind enum", func(r *contracts.DetectionRequest) { r.Column.Type.Kind = contracts.Kind(200) }},
		{"null column type", func(r *contracts.DetectionRequest) { r.Column.Type.Kind = contracts.Null }},
		{"wrong sample basis", func(r *contracts.DetectionRequest) { r.Sample.Basis = contracts.BasisUnknown }},
		{"bad date order", func(r *contracts.DetectionRequest) { r.DateOrder = "YMD" }},
		{"empty date order", func(r *contracts.DetectionRequest) { r.DateOrder = "" }},
		{"value kind mismatch", func(r *contracts.DetectionRequest) { r.Sample.Values = []contracts.Value{intValue} }},
		{"negative min samples", func(r *contracts.DetectionRequest) { r.MinSamples = -1 }},
		{"nan ratio", func(r *contracts.DetectionRequest) { r.MinRatio = math.NaN() }},
		{"positive inf ratio", func(r *contracts.DetectionRequest) { r.MinRatio = math.Inf(1) }},
		{"negative inf ratio", func(r *contracts.DetectionRequest) { r.MinRatio = math.Inf(-1) }},
		{"ratio below zero", func(r *contracts.DetectionRequest) { r.MinRatio = -0.1 }},
		{"ratio above one", func(r *contracts.DetectionRequest) { r.MinRatio = 1.1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testRequest(textValues(t, "a@example.com"))
			tc.set(&req)
			if _, err := New().Detect(context.Background(), req); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestDetectContextErrors(t *testing.T) {
	req := testRequest(textValues(t, "a@example.com"))
	if _, err := New().Detect(nil, req); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("nil context error = %v, want ErrInvalid", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Detect(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
	deadline, dcancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer dcancel()
	if _, err := New().Detect(deadline, req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline context error = %v", err)
	}
}

func TestDetectDoesNotMutateRequest(t *testing.T) {
	req := testRequest(textValues(t, append(emails(19), "not-an-email")...))
	req.Column.Ref.Column = "email"
	req.DateOrder = "DMY"
	originalColumn := req.Column
	originalValues := append([]contracts.Value(nil), req.Sample.Values...)
	originalBasis := req.Sample.Basis

	_ = detectOK(t, req)

	if req.Column != originalColumn {
		t.Fatal("column mutated")
	}
	if !reflect.DeepEqual(req.Sample.Values, originalValues) {
		t.Fatal("sample values mutated")
	}
	if req.Sample.Basis != originalBasis {
		t.Fatal("sample basis mutated")
	}
	if req.DateOrder != "DMY" {
		t.Fatal("date order mutated")
	}
}

func TestDetectReportsDoNotLeakRawSamples(t *testing.T) {
	const secret = "leak-marker-7f3a@example.com"
	values := textValues(t, secret)
	req := testRequest(values)

	reqJSON, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(reqJSON), "leak-marker-7f3a") {
		t.Fatal("request JSON leaked the sample")
	}
	if !strings.Contains(string(reqJSON), "Basis") {
		t.Fatal("unexpected request JSON shape")
	}

	valueJSON, err := json.Marshal(values[0])
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	if string(valueJSON) != `"[REDACTED]"` {
		t.Fatalf("value JSON = %s, want redacted", valueJSON)
	}

	req.MinSamples = 1
	decisionJSON, err := json.Marshal(detectOK(t, req))
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	if strings.Contains(string(decisionJSON), "leak-marker-7f3a") {
		t.Fatal("decision JSON leaked the sample")
	}
}

func TestDetectNullBlankAndTypeValidation(t *testing.T) {
	values := textValues(t, "a@example.com", "a@example.com", "  ")
	values = append(values, contracts.Value{})
	r := detectOK(t, testRequest(values))
	if r.Sensitivity != contracts.Unknown {
		t.Fatalf("one distinct non-blank sample must be insufficient: %#v", r)
	}
	ev := evidenceByRule(t, r, "email")
	if ev.Hits != 1 || ev.Total != 1 {
		t.Fatalf("evidence = %#v, want 1/1", ev)
	}
	if !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
		t.Fatalf("reasons = %v, want insufficient_evidence", ev.Reasons)
	}

	intValue, err := contracts.NewValue(contracts.Int, "1")
	if err != nil {
		t.Fatalf("NewValue(Int) = %v", err)
	}
	if _, err := New().Detect(context.Background(), testRequest([]contracts.Value{intValue})); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("type mismatch error = %v, want ErrInvalid", err)
	}
}
