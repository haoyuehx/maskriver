package detect

import (
	"context"
	"fmt"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func v(t *testing.T, kind contracts.Kind, raw string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(kind, raw)
	if err != nil {
		t.Fatalf("NewValue(%v,%q) err=%v", kind, raw, err)
	}
	return v
}

func sample(basis contracts.SampleBasis, vs ...contracts.Value) contracts.Sample {
	return contracts.Sample{Basis: basis, Values: vs, Truncated: false}
}

func basicCol(kind contracts.Kind) contracts.Column {
	return contracts.Column{
		Ref: contracts.ColumnRef{
			TableRef: contracts.TableRef{Database: "ds", Schema: "main", Table: "t"},
			Column:   "c",
		},
		Type: contracts.Type{Kind: kind, Native: "VARCHAR(255)", Nullable: true},
	}
}

func Test_Detector_Contract(t *testing.T) {
	d := New()
	if _, ok := d.(contracts.Detector); !ok {
		t.Fatalf("New() does not implement contracts.Detector")
	}
	// Zero-values Detect must not panic; unknown request yields safe error.
	_, err := d.Detect(context.Background(), contracts.DetectionRequest{})
	if err == nil {
		t.Fatalf("zero-request Detect should err")
	}
}

func Test_Detector_Precedence_Override_Reviewed_Skip(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	s := sample(contracts.DistinctNonNull, v(t, contracts.Text, "u@example.com"), v(t, contracts.Text, "a@b.co"))
	// Override.
	dec, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: s, MinSamples: 2, MinRatio: 0.5,
		Override: &contracts.Decision{Sensitivity: contracts.NotSensitive, RuleID: "whitelist"},
	})
	if err != nil {
		t.Fatalf("override err=%v", err)
	}
	if dec.Sensitivity != contracts.NotSensitive || dec.Source != contracts.Manual {
		t.Fatalf("override not honored: %+v", dec)
	}
	// Reviewed.
	dec, err = d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample(contracts.DistinctNonNull, v(t, contracts.Text, "123-45-6789")),
		MinSamples: 1, MinRatio: 0.99,
		Reviewed: &contracts.Decision{Sensitivity: contracts.Sensitive, RuleID: "manual"},
	})
	if err != nil || dec.Sensitivity != contracts.Sensitive || dec.Source != contracts.ReviewedHistory {
		t.Fatalf("reviewed not honored: %+v err=%v", dec, err)
	}
	// Skip.
	dec, err = d.Detect(ctx, contracts.DetectionRequest{Column: col, Sample: s, Skip: true})
	if err != nil || dec.Sensitivity != contracts.NotSensitive || dec.Source != contracts.Skip {
		t.Fatalf("skip not honored: %+v err=%v", dec, err)
	}
}

func Test_Detector_AllRules_Positive(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	cases := []struct {
		rule ruleID
		val  string
	}{
		{ruleEmail, "user.name+tag@example.co.uk"},
		{ruleURL, "https://example.com/path?q=1"},
		{ruleIPv4, "192.168.0.1"},
		{ruleUUID, "550e8400-e29b-41d4-a716-446655440000"},
		{ruleSSN, "123-45-6789"},
		{ruleCreditCard, "4111-1111-1111-1111"},
		{ruleCreditCard, "3782 822463 10005"},
		{ruleZipCode, "94107"},
		{ruleZipCode, "94107-1234"},
		{rulePhone, "(415) 555-2671"},
		{rulePhone, "1-415-555-2671"},
		{ruleAddress, "1600 Amphitheatre Pkwy"},
		{ruleCity, "San Francisco, CA"},
		{ruleFullName, "Jane Doe"},
		{ruleFullName, "Mary Ann O'Connor"},
		{ruleDate, "2026-01-02"},
		{ruleDate, "01/02/2026"},
		{ruleDate, "31-12-2026"},
	}
	for _, c := range cases {
		spec, ok := detectorRuleTable[c.rule]
		if !ok {
			t.Fatalf("missing rule %q", c.rule)
		}
		if !spec.match(c.val) {
			t.Errorf("rule %q positive miss: %q", c.rule, c.val)
		}
		dec, err := d.Detect(ctx, contracts.DetectionRequest{
			Column:     col,
			Sample:     sample(contracts.DistinctNonNull, v(t, contracts.Text, c.val)),
			MinSamples: 1, MinRatio: 0.99,
		})
		if err != nil {
			t.Fatalf("rule %q inspect err=%v", c.rule, err)
		}
		if dec.Sensitivity != contracts.Sensitive {
			t.Errorf("rule %q val %q sensitivity=%v want Sensitive (dec=%+v)", c.rule, c.val, dec.Sensitivity, dec)
		}
	}
}

func Test_Detector_AllRules_Negative(t *testing.T) {
	cases := []struct {
		rule ruleID
		val  string
	}{
		{ruleEmail, "no-at-sign"},
		{ruleURL, "example.com"},
		{ruleIPv4, "256.1.1.1"},
		{ruleIPv4, "01.01.01.01"},
		{ruleUUID, "not-a-uuid"},
		{ruleSSN, "000-00-0000"},
		{ruleSSN, "123456789"},
		{ruleCreditCard, "1234"},
		{ruleCreditCard, "4111-1111-1111-1112"},
		{ruleZipCode, "1234"},
		{rulePhone, "+44 20 7946 0018"},
		{ruleAddress, "Amphitheatre Pkwy"},
		{ruleCity, "NonExistentTown"},
		{ruleFullName, "jane doe"},
		{ruleFullName, "jane"},
		{ruleDate, "2026/01/02"},
	}
	for _, c := range cases {
		spec, ok := detectorRuleTable[c.rule]
		if !ok {
			t.Fatalf("missing rule %q", c.rule)
		}
		if spec.match(c.val) {
			t.Errorf("rule %q negative false-match: %q", c.rule, c.val)
		}
	}
}

func Test_Detector_SmallSampleSize_Unknown_Not_NotSensitive(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	// 19 hits vs MinSamples=20 → must be Unknown, not NotSensitive.
	values := make([]contracts.Value, 0, 20)
	for i := 0; i < 19; i++ {
		values = append(values, v(t, contracts.Text, "4111-1111-1111-1111"))
	}
	values = append(values, v(t, contracts.Text, "not-a-cc"))
	dec, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample(contracts.DistinctNonNull, values...),
		MinSamples: 21, MinRatio: 0.9,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if dec.Sensitivity != contracts.Unknown {
		t.Fatalf("expected Unknown for ineligible samples, got %+v", dec)
	}
	if len(dec.Evidence) == 0 || dec.Evidence[0].Eligible {
		t.Fatalf("expected Eligible=false for 19<21; evidence=%+v", dec.Evidence)
	}
	// Sample count exactly meets threshold.
	dec2, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample(contracts.DistinctNonNull, values...),
		MinSamples: 20, MinRatio: 1.0,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// Meets eligibility but 19/20 < 1.0 → Unknown fallback or NotSensitive based on absence of ambiguity.
	// Either way it must NOT be Sensitive.
	if dec2.Sensitivity == contracts.Sensitive {
		t.Fatalf("19/20 should not reach Sensitive at MinRatio=1.0: %+v", dec2)
	}
}

func Test_Detector_Ratio90_Sensitive(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	vals := make([]contracts.Value, 0, 20)
	for i := 0; i < 18; i++ {
		vals = append(vals, v(t, contracts.Text, fmt.Sprintf("u%d@example.com", i)))
	}
	vals = append(vals, v(t, contracts.Text, "a"), v(t, contracts.Text, "b"))
	dec, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample(contracts.DistinctNonNull, vals...),
		MinSamples: 10, MinRatio: 0.9,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if dec.Sensitivity != contracts.Sensitive {
		t.Fatalf("18/20 should meet 0.9 Sensitive threshold, got %+v", dec)
	}
	var emailEvidence *contracts.Evidence
	for i := range dec.Evidence {
		e := &dec.Evidence[i]
		if e.RuleID == "email" && e.Hits == 18 && e.Total == 20 && e.Threshold == 0.9 && e.Eligible {
			emailEvidence = e
			break
		}
	}
	if emailEvidence == nil {
		t.Fatalf("missing expected email evidence in %+v", dec.Evidence)
	}
}

func Test_Detector_EmptyNULL_Unknown(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	s := sample(contracts.DistinctNonNull, contracts.Value{}, contracts.Value{})
	dec, err := d.Detect(ctx, contracts.DetectionRequest{Column: col, Sample: s, MinSamples: 1, MinRatio: 0.5})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if dec.Sensitivity != contracts.Unknown {
		t.Fatalf("expected Unknown for NULL-only sample, got %+v", dec)
	}
}

func Test_Detector_DigitsOnly_NoFPRule(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	vals := make([]contracts.Value, 0, 10)
	for i := 0; i < 10; i++ {
		vals = append(vals, v(t, contracts.Text, fmt.Sprintf("%d", 100000+i)))
	}
	dec, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample(contracts.DistinctNonNull, vals...),
		MinSamples: 5, MinRatio: 0.5,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if dec.Sensitivity == contracts.Sensitive {
		t.Fatalf("pure numeric samples must not trigger aggregate Sensitive: %+v", dec)
	}
}

func Test_Detector_DateOrder_DMY(t *testing.T) {
	d := New()
	ctx := context.Background()
	col := basicCol(contracts.Text)
	values := make([]contracts.Value, 0, 20)
	// 18 unambiguous DMY dates (day > 12) and two ambiguous (13/12 → DMY valid, 12/12 → ambiguous).
	for i := 0; i < 18; i++ {
		values = append(values, v(t, contracts.Text, fmt.Sprintf("%02d/12/2026", 13+(i%16))))
	}
	values = append(values, v(t, contracts.Text, "12/12/2026"), v(t, contracts.Text, "11/12/2026"))
	dec, err := d.Detect(ctx, contracts.DetectionRequest{
		Column: col, DateOrder: "DMY",
		Sample:     sample(contracts.DistinctNonNull, values...),
		MinSamples: 10, MinRatio: 0.8,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// 18 out of 20 strictly DMY valid = 0.9 >= 0.8 → Sensitive allowed.
	if dec.Sensitivity != contracts.Sensitive {
		t.Fatalf("expected Sensitive at DMY 18/20 >= 0.8; got %+v", dec)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	n := 0
	for i > 0 {
		buf[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	if neg {
		buf[n] = '-'
		n++
	}
	for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}
	return string(buf[:n])
}
