package mask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestMaskRejectsNilAndCanceledContexts(t *testing.T) {
	input := newTextValue(t, "synthetic")
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	for _, ref := range []contracts.StrategyRef{refNull, refBlank, refRedact, refFormatRandom, refFakeEmail} {
		s, err := NewStrategy(ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Mask(nil, input, mc); !errors.Is(err, contracts.ErrInvalid) {
			t.Errorf("nil ctx for %s: %v", ref.ID, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.Mask(ctx, input, mc); !errors.Is(err, context.Canceled) {
			t.Errorf("canceled ctx for %s: %v", ref.ID, err)
		}
	}
	mapping := NewMemoryMapping()
	if _, _, err := mapping.Lookup(nil, contracts.MappingKey{}); !errors.Is(err, contracts.ErrInvalid) {
		t.Errorf("mapping Lookup(nil): %v", err)
	}
	if _, err := mapping.GetOrCreate(nil, contracts.MappingKey{}, input); !errors.Is(err, contracts.ErrInvalid) {
		t.Errorf("mapping GetOrCreate(nil): %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := mapping.Lookup(ctx, contracts.MappingKey{}); !errors.Is(err, context.Canceled) {
		t.Errorf("mapping canceled Lookup: %v", err)
	}
}

func TestMaskRejectsUnchangedSensitiveValues(t *testing.T) {
	ctx := context.Background()
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	redact, _ := NewStrategy(refRedact)
	for _, tc := range []struct{ kind contracts.Kind; raw string }{
		{contracts.Text, "***REDACTED***"},
		{contracts.Int, "0"},
		{contracts.Bool, "false"},
	} {
		input, err := contracts.NewValue(tc.kind, tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := redact.Mask(ctx, input, mc); !errors.Is(err, contracts.ErrUnsafe) {
			t.Errorf("unchanged redact for kind %v: %v", tc.kind, err)
		}
	}
	null, _ := NewStrategy(refNull)
	notNullable := mc
	notNullable.Plan.Type.Nullable = false
	if _, err := null.Mask(ctx, newTextValue(t, "x"), notNullable); !errors.Is(err, contracts.ErrUnsafe) {
		t.Errorf("null on NOT NULL column must fail: %v", err)
	}
	blank, _ := NewStrategy(refBlank)
	if _, err := blank.Mask(ctx, newIntValue(t, 1), mc); !errors.Is(err, contracts.ErrUnsupported) {
		t.Errorf("blank numeric type must fail: %v", err)
	}
}

func TestDeterministicFormatRandomPreservesBoundedShapes(t *testing.T) {
	ctx := context.Background()
	s, _ := NewStrategy(refFormatRandom)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	for _, input := range []string{"0", "a", "A", "Hello-129", "中文ABC-123", "北京 上海", strings.Repeat("a", 128)} {
		in := newTextValue(t, input)
		out, err := s.Mask(ctx, in, mc)
		if err != nil {
			t.Fatalf("supported text unexpectedly failed: %v", err)
		}
		if out.Kind() != contracts.Text || out.Payload() == input {
			t.Fatal("text format mask unchanged or wrong kind")
		}
		if utf8.RuneCountInString(out.Payload()) != utf8.RuneCountInString(input) || len(out.Payload()) != len(input) {
			t.Fatal("text rune/byte length not preserved")
		}
		again, err := s.Mask(ctx, in, mc)
		if err != nil || again != out {
			t.Fatal("text masking not deterministic")
		}
	}
	for _, input := range []string{"\x00", "\xff\x00", "a", strings.Repeat("\xff", 129)} {
		in := newBytesValue(t, input)
		out, err := s.Mask(ctx, in, mc)
		if err != nil || out.Kind() != contracts.Bytes || out.Payload() == input || len(out.Payload()) != len(input) {
			t.Fatal("byte masking must change payload and preserve Kind/length", err)
		}
	}
	for _, input := range []string{"!?.-", "é"} {
		if _, err := s.Mask(ctx, newTextValue(t, input), mc); !errors.Is(err, contracts.ErrUnsafe) && !errors.Is(err, contracts.ErrUnsupported) {
			t.Errorf("unsupported or unchangeable shape accepted")
		}
	}
}

func TestMaskRejectsTextKeysAndWrongPlanStrategy(t *testing.T) {
	ctx := context.Background()
	k := newTextValue(t, strings.Repeat("K", 40))
	for _, ref := range []contracts.StrategyRef{refFormatRandom, refFakeEmail} {
		s, _ := NewStrategy(ref)
		mc := contracts.MaskContext{Plan: basicPlan(), Key: k}
		if _, err := s.Mask(ctx, newTextValue(t, "sample"), mc); !errors.Is(err, contracts.ErrInvalid) {
			t.Errorf("text key accepted for %s", ref.ID)
		}
		mc.Key = testKey()
		mc.Plan.Strategy = refRedact
		if _, err := s.Mask(ctx, newTextValue(t, "sample"), mc); !errors.Is(err, contracts.ErrInvalid) {
			t.Errorf("wrong strategy plan accepted for %s", ref.ID)
		}
	}
}

func TestFakeEmailHasNoUnkeyedCRCTag(t *testing.T) {
	s, _ := NewStrategy(refFakeEmail)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	out, err := s.Mask(context.Background(), newTextValue(t, "synthetic@example.invalid"), mc)
	if err != nil {
		t.Fatal(err)
	}
	local, domain, ok := strings.Cut(out.Payload(), "@")
	if !ok || domain != maskDomain || len(local) != 32 {
		t.Fatal("expected only 16 bytes of keyed HMAC in local part")
	}
}
