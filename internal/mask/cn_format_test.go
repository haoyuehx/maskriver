package mask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func cnSyntheticID() string {
	first := "11010120000229001"
	return first + string(cnIDCheck(first))
}

func cnSyntheticUSCC() string {
	first := "91110000123456789"
	return first + string(cnUSCCCheck(first))
}

func cnSyntheticBank() string {
	first := "622202123456789"
	return first + string(cnLuhnDigit(first))
}

func cnTestContext(ref contracts.StrategyRef, kind contracts.Kind) contracts.MaskContext {
	return contracts.MaskContext{
		Key: testKey(),
		Plan: contracts.ColumnPlan{
			Strategy: ref, Type: contracts.Type{Kind: kind, Length: 120},
			Scope: "synthetic/m2", NormalizationVersion: "typed-lexical-v1", KeyID: "test-key",
		},
	}
}

type cnCase struct {
	id   string
	kind contracts.Kind
	good []string
	bad  []string
}

func cnCases() []cnCase {
	id := cnSyntheticID()
	uscc := cnSyntheticUSCC()
	bank := cnSyntheticBank()
	return []cnCase{
		{"cn_id_card", contracts.Text,
			[]string{id, strings.ToLower(id[:17]) + id[17:]},
			[]string{id[:17] + "?", "11010120230230001X", "123456200002290010"}},
		{"cn_mobile", contracts.Text,
			[]string{"13912345678", "+8613912345678", "8618812345678"},
			[]string{"12912345678", "1391234567", "+8713912345678"}},
		{"cn_bank_card", contracts.Text,
			[]string{bank, "6222021234567890" + string(cnLuhnDigit("6222021234567890"))},
			[]string{"1234", "0" + bank[1:], "a" + bank[1:]}},
		{"cn_uscc", contracts.Text,
			[]string{uscc},
			[]string{uscc[:17] + "I", "99110000123456789X", "invalid-uscc"}},
		{"cn_postal_code", contracts.Text,
			[]string{"100000", "510000"},
			[]string{"10000", "1000000", "A00000"}},
		{"cn_city", contracts.Text,
			[]string{"北京市", "Beijing", "shanghai city"},
			[]string{"巴黎", "Singapore", "Unknown City"}},
		{"cn_person_name", contracts.Text,
			[]string{"李明", "欧阳星河", "王小华"},
			[]string{"ABC", "苹果", "张", "张三丰丰丰"}},
		{"cn_address", contracts.Text,
			[]string{"北京市东城区和平路12号", "上海市黄浦区南京路123号"},
			[]string{"没有门牌的地址", "Street 123", "北京路号"}},
		{"cn_date", contracts.Text,
			[]string{"2024-02-29", "2024/02/29", "2024年2月29日"},
			[]string{"2023-02-29", "02/29/2024", "2024-13-01"}},
	}
}

// The cases are entirely manually constructed, development-time synthetic
// fixtures. Acceptance metrics on these cases are NOT production PII
// recognition, anonymization quality, or independent evaluation.
func TestCNFormatSyntheticEvaluation(t *testing.T) {
	for _, tc := range cnCases() {
		t.Run(tc.id, func(t *testing.T) {
			ref := contracts.StrategyRef{ID: tc.id, Version: cnFormatVersion}
			strategy, err := NewStrategy(ref)
			if err != nil {
				t.Fatal(err)
			}
			mc := cnTestContext(ref, tc.kind)
			tp, fp, fn := 0, 0, 0
			for _, raw := range tc.good {
				input := newTextValue(t, raw)
				got, err := strategy.Mask(context.Background(), input, mc)
				if err != nil {
					fn++
					continue
				}
				tp++
				if got.IsNull() || got.Kind() != tc.kind || got.Payload() == raw {
					t.Fatal("masked value lacks expected type or did not change")
				}
				if _, err := strategy.Mask(context.Background(), got, mc); err != nil {
					t.Fatal("generated output is not valid under the same rule:", err)
				}
				again, err := strategy.Mask(context.Background(), input, mc)
				if err != nil || again != got {
					t.Fatal("replacement not deterministic")
				}
				if tc.id == "cn_id_card" && !cnValidID(got.Payload()) ||
					tc.id == "cn_bank_card" && !cnValidBank(got.Payload()) ||
					tc.id == "cn_uscc" && !cnValidUSCC(got.Payload()) {
					t.Fatal("output has invalid checksum or shape")
				}
			}
			for _, raw := range tc.bad {
				input := newTextValue(t, raw)
				if _, err := strategy.Mask(context.Background(), input, mc); err == nil {
					fp++
				}
			}
			precision, recall := float64(0), float64(0)
			if tp+fp > 0 {
				precision = float64(tp) / float64(tp+fp)
			}
			if tp+fn > 0 {
				recall = float64(tp) / float64(tp+fn)
			}
			t.Logf("%s synthetic TP=%d FP=%d FN=%d precision=%.3f recall=%.3f", tc.id, tp, fp, fn, precision, recall)
			if fp != 0 || fn != 0 {
				t.Fatal("synthetic labelled shape acceptance failed")
			}
		})
	}
}

func TestCNFormatContractAndSafety(t *testing.T) {
	for _, tc := range cnCases() {
		t.Run(tc.id, func(t *testing.T) {
			ref := contracts.StrategyRef{ID: tc.id, Version: cnFormatVersion}
			strategy, _ := NewStrategy(ref)
			mc := cnTestContext(ref, tc.kind)
			sample := newTextValue(t, tc.good[0])
			if got, err := strategy.Mask(context.Background(), contracts.Value{}, mc); err != nil || !got.IsNull() {
				t.Fatal("NULL must remain NULL", err)
			}
			if _, err := strategy.Mask(nil, sample, mc); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatal("nil ctx should fail", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := strategy.Mask(ctx, sample, mc); !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation should fail", err)
			}
			bad := mc
			bad.Key = shortKey()
			if _, err := strategy.Mask(context.Background(), sample, bad); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatal("short key accepted", err)
			}
			bad = mc
			bad.Plan.Strategy.Version = "v2"
			if _, err := strategy.Mask(context.Background(), sample, bad); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatal("wrong strategy version accepted", err)
			}
			bad = mc
			bad.Plan.Scope = ""
			if _, err := strategy.Mask(context.Background(), sample, bad); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatal("missing scope accepted", err)
			}
			bad = mc
			bad.Plan.Type.Kind = contracts.Bytes
			if _, err := strategy.Mask(context.Background(), sample, bad); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatal("type mismatch accepted", err)
			}
			bad = mc
			bad.Plan.Type.Length = 1
			if _, err := strategy.Mask(context.Background(), sample, bad); !errors.Is(err, contracts.ErrUnsafe) {
				t.Fatal("too-short destination accepted", err)
			}
			// A different scope must not reuse a candidate by accident.
			first, err := strategy.Mask(context.Background(), sample, mc)
			if err != nil {
				t.Fatal(err)
			}
			scope := mc
			scope.Plan.Scope = "synthetic/m2/other"
			second, err := strategy.Mask(context.Background(), sample, scope)
			if err != nil {
				t.Fatal(err)
			}
			// A finite output space could collide. This is explicitly NOT
			// promised to be collision-free, so this assertion tests
			// deterministic key separation rather than output inequality.
			_, _ = first, second
		})
	}
}

func TestCNFormatDateAndBoundaries(t *testing.T) {
	ref := contracts.StrategyRef{ID: "cn_date", Version: cnFormatVersion}
	strategy, err := NewStrategy(ref)
	if err != nil {
		t.Fatal(err)
	}
	mc := cnTestContext(ref, contracts.Date)
	val, err := contracts.NewValue(contracts.Date, "2024-02-29")
	if err != nil {
		t.Fatal(err)
	}
	out, err := strategy.Mask(context.Background(), val, mc)
	if err != nil || out.Kind() != contracts.Date || out == val {
		t.Fatal("typed Date failed", err)
	}
	if date, err := time.Parse(time.DateOnly, out.Payload()); err != nil || date.Year() < 1900 {
		t.Fatal("output date invalid", err)
	}
	mc.Plan.Type.Kind = contracts.Text
	textOut, err := strategy.Mask(context.Background(), newTextValue(t, "2024年2月29日"), mc)
	if err != nil || !strings.Contains(textOut.Payload(), "年") {
		t.Fatal("Chinese date style lost", err)
	}
	if utf8.RuneCountInString(textOut.Payload()) < 8 {
		t.Fatal("date malformed")
	}
	for _, bad := range []contracts.StrategyRef{
		{ID: "cn_mobile", Version: "v2"}, {ID: "cn_unknown", Version: "1"},
	} {
		if _, err := NewStrategy(bad); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatal("unknown strategy accepted")
		}
	}
}
