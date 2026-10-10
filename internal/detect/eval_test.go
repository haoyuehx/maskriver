package detect

import (
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file is the labeled synthetic evaluation for the detector. It is part of
// the test suite (no external dataset, no runner, no I/O) and reports a
// per-rule confusion summary when run with `go test -v ./internal/detect`.
//
// Unit of evaluation. Each evalSample is a (value, column-name) pair, so the
// evaluation measures the detector's field-aware decision for one value at a
// time. DetectionRequest.MinSamples is set to 1 and the default MinRatio 0.9
// holds, so a single matching value is 1/1 = 1.0 >= 0.9 and is eligible on its
// own. This is deliberately different from the production default of 20
// distinct values: the value+field unit isolates per-rule rule shape and
// column-context behavior, while the column-level aggregation default is tested
// separately (for example TestCnGeoColumnAggregationDefaultThreshold and
// TestDetectEmailThresholdBoundary).
//
// Honesty of the labels. Every rule has in-coverage positives (asserted to be
// detected), declared out-of-coverage positives (real-world positives that the
// bounded rule deliberately does not cover; they are counted as false negatives
// so recall is not inflated), and confusing negatives (near-miss or ambiguous
// values; a false positive is reported rather than hidden). Ordinary words that
// start with a listed surname (李子/白天/石头) are included as negatives so the
// cn_person_name false positives they cause are measured, not hidden.

// evalSample is one labeled (value, column) observation.
type evalSample struct {
	rule           string // expected rule for positives; resembles-rule for negatives
	column         string
	value          string
	positive       bool
	expectDetected bool // in-coverage positive that must classify as rule
	outOfCoverage  bool // declared positive outside the bounded rule coverage
	note           string
}

// evalRuleIDs returns the 13 rules covered by the evaluation: the A/B rules
// already implemented plus the C rules added for issue #6.
func evalRuleIDs() []string {
	return []string{
		"email", "url", "ip_address", "uuid", "cn_date",
		"cn_id_card", "cn_mobile", "cn_bank_card", "cn_postal_code", "cn_uscc",
		"cn_city", "cn_person_name", "cn_address",
	}
}

// evalDataset returns the labeled synthetic samples. Values are self-generated
// fixtures only; no real personal data is used. Checked values reuse the local
// generators in cn_test.go, which are independent of the rule checksum code.
func evalDataset() []evalSample {
	card16 := genTestBankCard("6200000000000", 16)
	return []evalSample{
		// A rules: universal formats.
		{rule: "email", column: "c", value: "user01@example.com", positive: true, expectDetected: true},
		{rule: "email", column: "c", value: "first.last+tag@sub.example.co.uk", positive: true, expectDetected: true},
		{rule: "email", column: "c", value: "user01.example.com", note: "missing at-sign"},
		{rule: "email", column: "c", value: "a@example", note: "missing dotted tld"},

		{rule: "url", column: "c", value: "https://www.example.cn/path?q=1#top", positive: true, expectDetected: true},
		{rule: "url", column: "c", value: "example.com", note: "host without scheme"},

		{rule: "ip_address", column: "c", value: "192.168.10.20", positive: true, expectDetected: true},
		{rule: "ip_address", column: "c", value: "2001:db8::1", positive: true, expectDetected: true},
		{rule: "ip_address", column: "c", value: "256.1.1.1", note: "out-of-range octet"},

		{rule: "uuid", column: "c", value: "550e8400-e29b-41d4-a716-446655440000", positive: true, expectDetected: true},
		{rule: "uuid", column: "c", value: "550e8400-e29b-01d4-a716-446655440000", note: "version 0"},

		{rule: "cn_date", column: "c", value: "2023年1月2日", positive: true, expectDetected: true},
		{rule: "cn_date", column: "c", value: "2024-02-29", positive: true, expectDetected: true},
		{rule: "cn_date", column: "c", value: "2023-02-29", note: "non-leap February 29"},

		// B rules: bounded China-first identifiers.
		{rule: "cn_id_card", column: "c", value: genTestID("110101", "19900101", 1), positive: true, expectDetected: true},
		{rule: "cn_id_card", column: "c", value: genTestID("440304", "19851231", 999), positive: true, expectDetected: true},
		{rule: "cn_id_card", column: "c", value: genTestID("110102", "19900101", 1), positive: true, outOfCoverage: true, note: "real area outside the 15-code subset"},
		{rule: "cn_id_card", column: "c", value: "110101199001010016", note: "bad check character"},
		{rule: "cn_id_card", column: "c", value: "999999199001010015", note: "impossible area"},

		{rule: "cn_mobile", column: "phone", value: "13800138000", positive: true, expectDetected: true},
		{rule: "cn_mobile", column: "mobile", value: "+8613800138000", positive: true, expectDetected: true},
		{rule: "cn_mobile", column: "phone", value: "19212345678", positive: true, outOfCoverage: true, note: "real 192 segment outside the conservative prefix subset"},
		{rule: "cn_mobile", column: "order_number", value: "13800138000", note: "phone shape under a non-phone column"},
		{rule: "cn_mobile", column: "phone", value: "12345678901", note: "unsupported prefix"},

		{rule: "cn_bank_card", column: "bank_card", value: card16, positive: true, expectDetected: true},
		{rule: "cn_bank_card", column: "bank_card", value: "1234567890123456", note: "fails Luhn"},

		{rule: "cn_postal_code", column: "postal_code", value: "100000", positive: true, expectDetected: true},
		{rule: "cn_postal_code", column: "邮编", value: "518000", positive: true, expectDetected: true},
		{rule: "cn_postal_code", column: "postal_code", value: "10000", note: "five digits"},
		{rule: "cn_postal_code", column: "c", value: "100000", note: "six digits without context"},

		{rule: "cn_uscc", column: "c", value: "9144010000000000A3", positive: true, expectDetected: true},
		{rule: "cn_uscc", column: "c", value: genTestUSCC("91", "370100", "00000000A"), positive: true, outOfCoverage: true, note: "real area outside the USCC subset"},
		{rule: "cn_uscc", column: "c", value: "9144010000000000A4", note: "bad check character"},

		// C rules: bounded geographic names, personal names, addresses.
		{rule: "cn_city", column: "city", value: "北京", positive: true, expectDetected: true},
		{rule: "cn_city", column: "城市", value: "北京市", positive: true, expectDetected: true},
		{rule: "cn_city", column: "city", value: "Shanghai", positive: true, expectDetected: true},
		{rule: "cn_city", column: "city", value: "Xi'an", positive: true, expectDetected: true},
		{rule: "cn_city", column: "province", value: "广东省", positive: true, expectDetected: true},
		{rule: "cn_city", column: "province", value: "内蒙古自治区", positive: true, expectDetected: true},
		{rule: "cn_city", column: "城市", value: "GUANGZHOU CITY", positive: true, expectDetected: true},
		{rule: "cn_city", column: "city", value: "武汉", positive: true, outOfCoverage: true, note: "real city outside the bounded list"},
		{rule: "cn_city", column: "city", value: "Nanjing", positive: true, outOfCoverage: true, note: "real city outside the bounded list"},
		{rule: "cn_city", column: "c", value: "Beijing", note: "geographic name without context"},
		{rule: "cn_city", column: "city", value: "marketing", note: "ordinary English word"},
		{rule: "cn_city", column: "name", value: "北京", note: "place name under a name column"},

		{rule: "cn_person_name", column: "姓名", value: "王明", positive: true, expectDetected: true},
		{rule: "cn_person_name", column: "full_name", value: "李小明", positive: true, expectDetected: true},
		{rule: "cn_person_name", column: "姓名", value: "欧阳娜娜", positive: true, expectDetected: true},
		{rule: "cn_person_name", column: "姓名", value: "佘曼", positive: true, outOfCoverage: true, note: "surname outside the bounded subset"},
		{rule: "cn_person_name", column: "name", value: "普通文本", note: "ordinary four-Han text"},
		{rule: "cn_person_name", column: "name", value: "成都", note: "bounded geographic alias, declined by the name rule"},
		{rule: "cn_person_name", column: "name", value: "李子", note: "ordinary word starting with the 李 surname (known false positive)"},
		{rule: "cn_person_name", column: "name", value: "白天", note: "ordinary word starting with the 白 surname (known false positive)"},
		{rule: "cn_person_name", column: "name", value: "石头", note: "ordinary word starting with the 石 surname (known false positive)"},
		{rule: "cn_person_name", column: "name", value: "marketing", note: "English word under a name column"},

		{rule: "cn_address", column: "地址", value: "北京市朝阳区建国路88号", positive: true, expectDetected: true},
		{rule: "cn_address", column: "addr", value: "广东省深圳市南山区科技园南路15号3栋201室", positive: true, expectDetected: true},
		{rule: "cn_address", column: "addr", value: "中关村大街1号院3号楼", positive: true, expectDetected: true},
		{rule: "cn_address", column: "地址", value: "华夏大厦5层501室", positive: true, expectDetected: true},
		{rule: "cn_address", column: "地址", value: "3栋2单元501室", positive: true, outOfCoverage: true, note: "room-only fragment, insufficient markers"},
		{rule: "cn_address", column: "addr", value: "123 Main Street, Springfield", positive: true, outOfCoverage: true, note: "English address outside the rule"},
		{rule: "cn_address", column: "addr", value: "北京市", note: "administrative name only"},
		{rule: "cn_address", column: "addr", value: "这是一条路", note: "road word without house number"},
		{rule: "cn_address", column: "地址", value: "今天天气很好", note: "ordinary text"},
		{rule: "cn_address", column: "地址", value: "城市道路管理", note: "notice with admin and road words but no house component"},
		{rule: "cn_address", column: "地址", value: "社区1号公告", note: "notice with prose after a house component"},
		{rule: "cn_address", column: "地址", value: "街道通知", note: "notice with street words but no house component"},
		{rule: "cn_address", column: "addr", value: "ABC路1号", note: "foreign stem has no Han place name"},
		{rule: "cn_address", column: "地址", value: "区县1号", note: "administrative words without a place stem"},

		// Ordinary non-sensitive confusion for several rules at once.
		{rule: "", column: "c", value: "测试"},
		{rule: "", column: "c", value: "hello world"},
		{rule: "", column: "c", value: "OK"},
		{rule: "", column: "c", value: "abc123"},
		{rule: "", column: "c", value: "123456"},
	}
}

// evalCounts is a per-class confusion row.
type evalCounts struct{ tp, fp, fn, tn int }

func TestCnEvaluationLabeledSynthetic(t *testing.T) {
	rules := evalRuleIDs()
	samples := evalDataset()

	classCounts := make(map[string]*evalCounts, len(rules)+1)
	for _, rule := range rules {
		classCounts[rule] = &evalCounts{}
	}
	classCounts["unclassified"] = &evalCounts{}

	var positives, inCoverage, outOfCoverage, negatives int
	perRulePositive := make(map[string]int, len(rules))
	perRuleNegative := make(map[string]int, len(rules))

	for i, sample := range samples {
		req := testRequest(textValues(t, sample.value))
		req.Column.Ref.Column = sample.column
		// Per-value unit: a single matching value is 1/1 = 1.0 >= the default
		// 0.9 ratio, so the rule shape and column context are what is measured.
		req.MinSamples = 1
		decision := detectOK(t, req)

		// Abstention is recorded as its own class. Unknown is not a safe or
		// non-sensitive verdict; the one-vs-rest counts below only say that the
		// detector did not classify the sample as any bounded rule.
		predicted := "unclassified"
		if decision.Sensitivity == contracts.Sensitive {
			predicted = decision.RuleID
		}
		expected := "unclassified"
		if sample.positive {
			expected = sample.rule
		}

		if sample.positive {
			positives++
			perRulePositive[sample.rule]++
			if sample.outOfCoverage {
				outOfCoverage++
			} else {
				inCoverage++
			}
		} else {
			negatives++
			perRuleNegative[sample.rule]++
		}

		for class, counts := range classCounts {
			switch {
			case expected == class && predicted == class:
				counts.tp++
			case expected != class && predicted == class:
				counts.fp++
			case expected == class && predicted != class:
				counts.fn++
			default:
				counts.tn++
			}
		}

		if sample.expectDetected && (decision.Sensitivity != contracts.Sensitive || decision.RuleID != sample.rule) {
			t.Errorf("in-coverage positive sample %d (rule %s) was not detected as %s", i, sample.rule, sample.rule)
		}
	}

	// Structural assertions: every rule is exercised by at least one in-coverage
	// positive and one confusing negative, and the declared out-of-coverage set is
	// non-empty so recall is measured honestly.
	for _, rule := range rules {
		var ruleInCoverage int
		for _, sample := range samples {
			if sample.positive && sample.rule == rule && !sample.outOfCoverage {
				ruleInCoverage++
			}
		}
		if ruleInCoverage == 0 {
			t.Errorf("rule %s has no in-coverage positive sample", rule)
		}
		if perRuleNegative[rule] == 0 {
			t.Errorf("rule %s has no negative/confusion sample", rule)
		}
	}
	if outOfCoverage == 0 {
		t.Error("evaluation has no declared out-of-coverage positive samples")
	}

	t.Logf("dataset samples=%d positives=%d (in-coverage=%d out-of-coverage=%d) negatives=%d rules=%d",
		len(samples), positives, inCoverage, outOfCoverage, negatives, len(rules))
	t.Logf("%-14s %4s %4s %4s %9s %7s %7s %8s", "rule", "TP", "FP", "FN", "precision", "recall", "f1", "support")
	for _, rule := range append(rules, "unclassified") {
		counts := classCounts[rule]
		precision, recall, f1 := precisionRecallF1(counts.tp, counts.fp, counts.fn)
		t.Logf("%-14s %4d %4d %4d %9.3f %7.3f %7.3f %8d",
			rule, counts.tp, counts.fp, counts.fn, precision, recall, f1, counts.tp+counts.fn)
	}
	t.Logf("note: the unclassified row is one-vs-rest abstention, not a non-sensitive verdict; Unknown is never a safe result")

	// Measured limitations: out-of-coverage positives lower recall (they are
	// counted inside the FN column), and any confusion-driven false positive is
	// listed explicitly.
	for _, rule := range rules {
		counts := classCounts[rule]
		ooc := 0
		for _, sample := range samples {
			if sample.positive && sample.outOfCoverage && sample.rule == rule {
				ooc++
			}
		}
		if ooc > 0 {
			t.Logf("limitation: %s recall counts %d declared out-of-coverage positive(s) as false negatives", rule, ooc)
		}
		if counts.fp > 0 {
			t.Logf("limitation: %s has %d measured false positive(s) from confusing negatives", rule, counts.fp)
		}
	}
}

// precisionRecallF1 returns precision, recall, and F1 for a confusion row. A
// zero denominator yields zero rather than a NaN so the report stays numeric.
func precisionRecallF1(tp, fp, fn int) (precision, recall, f1 float64) {
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}
