package detect

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file is an independent, synthetic holdout for the M3-4 evaluation. It
// deliberately does not reuse the detector rules or the existing eval_test.go
// table: valid identifiers are generated from their published checksum
// algorithms, and negatives are mutations. No real personal data is used.
// It is not a human-adjudicated or production accuracy measurement.

type holdoutCase struct {
	rule     string
	column   string
	value    contracts.Value
	positive bool
}

func holdoutText(t *testing.T, s string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Text, s)
	if err != nil {
		t.Fatalf("holdout value failed: %v", err)
	}
	return v
}

func holdoutDate(t *testing.T, s string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Date, s)
	if err != nil {
		t.Fatalf("holdout date failed: %v", err)
	}
	return v
}

func idCheck(body string) byte {
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	const checks = "10X98765432"
	sum := 0
	for i, c := range []byte(body) {
		sum += int(c-'0') * weights[i]
	}
	return checks[sum%11]
}

func luhnCheck(body string) byte {
	sum := 0
	double := true
	for i := len(body) - 1; i >= 0; i-- {
		d := int(body[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return byte('0' + (10-sum%10)%10)
}

func usccCheck(body string) byte {
	const alphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	weights := []int{1, 3, 9, 27, 19, 26, 16, 17, 20, 29, 25, 13, 8, 24, 10, 30, 28}
	sum := 0
	for i, c := range []byte(body) {
		sum += strings.IndexByte(alphabet, c) * weights[i]
	}
	return alphabet[(31-sum%31)%31]
}

func holdoutCases(t *testing.T) []holdoutCase {
	t.Helper()
	var out []holdoutCase
	add := func(rule, column string, value contracts.Value, positive bool) {
		out = append(out, holdoutCase{rule: rule, column: column, value: value, positive: positive})
	}
	for i := 0; i < 20; i++ {
		add("email", "email", holdoutText(t, fmt.Sprintf("holdout%02d@example.invalid", i)), true)
		add("email", "email", holdoutText(t, fmt.Sprintf("holdout%02d.example.invalid", i)), false)
	}
	for i := 0; i < 12; i++ {
		add("url", "url", holdoutText(t, fmt.Sprintf("https://example.invalid/holdout/%d?q=%d", i, i)), true)
		add("url", "url", holdoutText(t, fmt.Sprintf("example.invalid/holdout/%d", i)), false)
	}
	for i := 0; i < 12; i++ {
		add("ip_address", "ip", holdoutText(t, fmt.Sprintf("10.%d.%d.%d", i, i+1, i+2)), true)
		add("ip_address", "ip", holdoutText(t, fmt.Sprintf("10.%d.%d", i, i+1)), false)
	}
	for i := 0; i < 12; i++ {
		add("uuid", "uuid", holdoutText(t, fmt.Sprintf("550e8400-e29b-41d4-a716-44665544%04d", i)), true)
		add("uuid", "uuid", holdoutText(t, fmt.Sprintf("550e8400-e29b-11d4-7716-44665544%04d", i)), false)
	}
	for i := 0; i < 20; i++ {
		d := time.Date(1980+i%30, time.Month(1+i%12), 1+i%27, 0, 0, 0, 0, time.UTC)
		add("cn_date", "born", holdoutDate(t, d.Format(time.DateOnly)), true)
	}
	for _, bad := range []string{"2023-02-29", "2023-13-01", "0000-01-01", "2023-00-10"} {
		add("cn_date", "born", holdoutText(t, bad), false)
	}
	for i := 0; i < 25; i++ {
		birth := time.Date(1970+i%40, time.Month(1+i%12), 1+i%27, 0, 0, 0, 0, time.UTC).Format("20060102")
		body := fmt.Sprintf("110101%s%03d", birth, i+1)
		add("cn_id_card", "identity", holdoutText(t, body+string(idCheck(body))), true)
		add("cn_id_card", "identity", holdoutText(t, body+string((idCheck(body)+1)%10+'0')), false)
	}
	prefixes := []string{"130", "131", "132", "133", "135", "136", "137", "138", "139", "150", "151", "152", "155", "156", "157", "158", "159", "176", "177", "180", "181", "182", "183", "185", "186", "187", "188", "189"}
	for i, prefix := range prefixes {
		add("cn_mobile", "mobile", holdoutText(t, fmt.Sprintf("%s%08d", prefix, 10000000+i)), true)
	}
	for _, bad := range []string{"10000000000", "1380000000", "138000000000", "1380000000x"} {
		add("cn_mobile", "mobile", holdoutText(t, bad), false)
	}
	for i := 0; i < 20; i++ {
		body := fmt.Sprintf("62%014d", 10000000000000+i)
		add("cn_bank_card", "bank_card", holdoutText(t, body+string(luhnCheck(body))), true)
		add("cn_bank_card", "bank_card", holdoutText(t, body+string((luhnCheck(body)+1)%10+'0')), false)
	}
	for i := 0; i < 20; i++ {
		add("cn_postal_code", "postal_code", holdoutText(t, fmt.Sprintf("%06d", 100000+i*137)), true)
	}
	for _, bad := range []string{"12345", "1234567", "00000a"} {
		add("cn_postal_code", "postal_code", holdoutText(t, bad), false)
	}
	const alphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	authority := []string{"110000", "120000", "310000", "500000", "440100", "440300", "330100", "320100", "510100", "420100", "610100", "350100"}
	for i := 0; i < 20; i++ {
		var b strings.Builder
		b.WriteString("91")
		b.WriteString(authority[i%len(authority)])
		for j := 0; j < 9; j++ {
			b.WriteByte(alphabet[(i*7+j*13)%len(alphabet)])
		}
		body := b.String()
		add("cn_uscc", "uscc", holdoutText(t, body+string(usccCheck(body))), true)
		add("cn_uscc", "uscc", holdoutText(t, body+string((usccCheck(body)+1)%10+'0')), false)
	}
	for _, city := range []string{"北京市", "上海市", "广州市", "深圳市", "杭州市", "成都市", "重庆市", "天津市", "广东省", "浙江省"} {
		add("cn_city", "city", holdoutText(t, city), true)
	}
	add("cn_city", "city", holdoutText(t, "Atlantis"), false)
	for _, name := range []string{"张伟", "王芳", "李娜", "刘洋", "陈静", "杨帆", "赵磊", "黄敏"} {
		add("cn_person_name", "name", holdoutText(t, name), true)
	}
	add("cn_person_name", "name", holdoutText(t, "Product Name"), false)
	for _, address := range []string{"北京市朝阳区建国路1号", "上海市浦东新区世纪大道2号", "广州市天河区体育西路3号"} {
		add("cn_address", "address", holdoutText(t, address), true)
	}
	add("cn_address", "address", holdoutText(t, "N/A"), false)
	return out
}

func TestHoldoutEvaluation(t *testing.T) {
	cases := holdoutCases(t)
	type metric struct{ tp, fp, fn int }
	metrics := map[string]*metric{}
	request := func(c holdoutCase) contracts.DetectionRequest {
		return contracts.DetectionRequest{
			Column: contracts.Column{
				Ref:  contracts.ColumnRef{TableRef: contracts.TableRef{Database: "holdout", Schema: "main", Table: "t"}, Column: c.column},
				Type: contracts.Type{Kind: contracts.Text},
			},
			Sample:     contracts.Sample{Values: []contracts.Value{c.value}, Basis: contracts.DistinctNonNull},
			MinSamples: 1, MinRatio: 1, DateOrder: "MDY",
		}
	}
	for _, c := range cases {
		if c.value.Kind() == contracts.Date {
			req := request(c)
			req.Column.Type.Kind = contracts.Date
			r, err := New().Detect(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			m := metrics[c.rule]
			if m == nil {
				m = &metric{}
				metrics[c.rule] = m
			}
			got := r.Sensitivity == contracts.Sensitive && r.RuleID == c.rule
			if c.positive && got {
				m.tp++
			} else if c.positive {
				m.fn++
				t.Logf("holdout FN rule=%s column=%s", c.rule, c.column)
			} else if got {
				m.fp++
				t.Logf("holdout FP rule=%s column=%s", c.rule, c.column)
			}
			continue
		}
		r, err := New().Detect(context.Background(), request(c))
		if err != nil {
			t.Fatal(err)
		}
		m := metrics[c.rule]
		if m == nil {
			m = &metric{}
			metrics[c.rule] = m
		}
		got := r.Sensitivity == contracts.Sensitive && r.RuleID == c.rule
		if c.positive && got {
			m.tp++
		} else if c.positive {
			m.fn++
			t.Logf("holdout FN rule=%s column=%s", c.rule, c.column)
		} else if got {
			m.fp++
			t.Logf("holdout FP rule=%s column=%s", c.rule, c.column)
		}
	}
	total := metric{}
	for _, rule := range []string{"email", "url", "ip_address", "uuid", "cn_date", "cn_id_card", "cn_mobile", "cn_bank_card", "cn_postal_code", "cn_uscc", "cn_city", "cn_person_name", "cn_address"} {
		m := metrics[rule]
		if m == nil {
			t.Fatalf("holdout rule missing: %s", rule)
		}
		total.tp += m.tp
		total.fp += m.fp
		total.fn += m.fn
		precision, recall := 1.0, 1.0
		if m.tp+m.fp > 0 {
			precision = float64(m.tp) / float64(m.tp+m.fp)
		}
		if m.tp+m.fn > 0 {
			recall = float64(m.tp) / float64(m.tp+m.fn)
		}
		t.Logf("holdout rule=%-14s tp=%3d fp=%2d fn=%2d precision=%.3f recall=%.3f", rule, m.tp, m.fp, m.fn, precision, recall)
	}
	p := float64(total.tp) / float64(total.tp+total.fp)
	r := float64(total.tp) / float64(total.tp+total.fn)
	t.Logf("holdout overall tp=%d fp=%d fn=%d precision=%.3f recall=%.3f", total.tp, total.fp, total.fn, p, r)
	if total.tp+total.fp == 0 || total.tp+total.fn == 0 {
		t.Fatal("holdout produced no evidence")
	}
	// This is a holdout measurement, not a tuning gate. Fail only if a large
	// share of generated positives is missed, which would invalidate the
	// recorded evidence as a positive result.
	if r < 0.90 || p < 0.90 {
		t.Fatalf("holdout quality below floor: precision=%.3f recall=%.3f", p, r)
	}
}
