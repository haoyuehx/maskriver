package mask

import (
	"context"
	"fmt"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

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
		{"cn_id_card", contracts.Text, cnValidID, func(i int) string {
			body := fmt.Sprintf("11010119900101%03d", (i%999)+1)
			return body + string(cnIDCheck(body))
		}},
		{"cn_bank_card", contracts.Text, cnValidBank, func(i int) string {
			body := fmt.Sprintf("622202%010d", 1000000000+i)
			return body + string(cnLuhnDigit(body))
		}},
		{"cn_uscc", contracts.Text, cnValidUSCC, func(i int) string {
			body := fmt.Sprintf("91110000%09d", 100000000+i)
			return body + string(cnUSCCCheck(body))
		}},
		{"cn_postal_code", contracts.Text, cnValidPostal, func(i int) string {
			return fmt.Sprintf("%06d", 100000+i)
		}},
		{"cn_mobile", contracts.Text, func(s string) bool { return len(s) == 11 }, func(i int) string {
			return fmt.Sprintf("138%08d", 10000000+i)
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
		if deterministic != 100 || shape != 100 || changed != 100 {
			t.Fatalf("%s failed holdout invariants", s.id)
		}
	}
}
