package mask

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Independent checksum/format helpers for the holdout. They are deliberately
// not the production cnIDCheck/cnLuhnDigit/cnUSCCCheck validators.
func holdoutIDCheck(body string) byte {
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	const checks = "10X98765432"
	sum := 0
	for i, c := range []byte(body) {
		sum += int(c-'0') * weights[i]
	}
	return checks[sum%11]
}

func holdoutLuhn(body string) byte {
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

func holdoutUSCC(body string) byte {
	const alphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	weights := []int{1, 3, 9, 27, 19, 26, 16, 17, 20, 29, 25, 13, 8, 24, 10, 30, 28}
	sum := 0
	for i, c := range []byte(body) {
		sum += strings.IndexByte(alphabet, c) * weights[i]
	}
	return alphabet[(31-sum%31)%31]
}

func holdoutIDValid(s string) bool {
	if len(s) != 18 || strings.IndexByte("10X98765432", s[17]) < 0 {
		return false
	}
	return s[17] == holdoutIDCheck(s[:17])
}

func holdoutBankValid(s string) bool {
	return len(s) >= 16 && len(s) <= 19 && s[0] != '0' && s[len(s)-1] == holdoutLuhn(s[:len(s)-1])
}

func holdoutUSCCValid(s string) bool {
	const alphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	return len(s) == 18 && strings.IndexByte(alphabet, s[17]) >= 0 && s[17] == holdoutUSCC(s[:17])
}

func holdoutPostalValid(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func holdoutMobileValid(s string) bool {
	if len(s) != 11 {
		return false
	}
	for i := 0; i < 11; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	prefixes := "|130|131|132|133|134|135|136|137|138|139|150|151|152|153|155|156|157|158|159|170|171|173|175|176|177|178|180|181|182|183|184|185|186|187|188|189|191|193|195|196|198|199|"
	return strings.Contains(prefixes, "|"+s[:3]+"|")
}

// M3-4 masking holdout: deterministic generated valid identifiers, distinct
// from the hand-built cnCases table. It measures shape preservation, determinism
// and output collisions. It is synthetic and not a production anonymity metric.
func TestMaskingHoldout(t *testing.T) {
	type suite struct {
		id    string
		kind  contracts.Kind
		valid func(string) bool
		build func(int) string
	}
	suites := []suite{
		{"cn_id_card", contracts.Text, holdoutIDValid, func(i int) string {
			body := fmt.Sprintf("1101011985%02d%02d%03d", (i%12)+1, (i%27)+1, (i%999)+1)
			return body + string(holdoutIDCheck(body))
		}},
		{"cn_bank_card", contracts.Text, holdoutBankValid, func(i int) string {
			body := fmt.Sprintf("622202%010d", 1000000000+i)
			return body + string(holdoutLuhn(body))
		}},
		{"cn_uscc", contracts.Text, holdoutUSCCValid, func(i int) string {
			body := fmt.Sprintf("91110000%09d", 100000000+i)
			return body + string(holdoutUSCC(body))
		}},
		{"cn_postal_code", contracts.Text, holdoutPostalValid, func(i int) string {
			return fmt.Sprintf("%06d", 100000+i)
		}},
		{"cn_mobile", contracts.Text, holdoutMobileValid, func(i int) string {
			prefixes := []string{"138", "139", "150", "151", "152", "155", "156", "157", "158", "159", "186", "187", "188", "189"}
			return fmt.Sprintf("%s%08d", prefixes[i%len(prefixes)], 10000000+i)
		}},
	}
	for _, s := range suites {
		ref := contracts.StrategyRef{ID: s.id, Version: cnFormatVersion}
		strategy, err := NewStrategy(ref)
		if err != nil {
			t.Fatal(err)
		}
		mc := cnTestContext(ref, s.kind)
		seen := map[string]bool{}
		deterministic, shape, changed := 0, 0, 0
		for i := 0; i < 100; i++ {
			raw := s.build(i)
			input := newTextValue(t, raw)
			got, err := strategy.Mask(context.Background(), input, mc)
			if err != nil || got.IsNull() || got.Kind() != s.kind {
				t.Fatalf("%s rejected generated input %d: %v", s.id, i, err)
			}
			again, err := strategy.Mask(context.Background(), input, mc)
			if err == nil && again == got {
				deterministic++
			}
			if s.valid(got.Payload()) {
				shape++
			}
			if got.Payload() != raw {
				changed++
			}
			seen[got.Payload()] = true
		}
		t.Logf("mask holdout rule=%-14s inputs=100 deterministic=%d valid_shape=%d changed=%d unique_outputs=%d",
			s.id, deterministic, shape, changed, len(seen))
		if deterministic != 100 || shape != 100 || changed != 100 || len(seen) != 100 {
			t.Fatalf("%s failed holdout invariants", s.id)
		}
	}
}
