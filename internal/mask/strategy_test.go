package mask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func testMaskContext(t *testing.T, id string, kind contracts.Kind) contracts.MaskContext {
	t.Helper()
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("k", minimumKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	return contracts.MaskContext{
		Plan: contracts.ColumnPlan{
			Type:                 contracts.Type{Kind: kind, Nullable: true, Length: 128},
			Strategy:             contracts.StrategyRef{ID: id, Version: strategyVersion},
			Scope:                "synthetic-scope",
			NormalizationVersion: normalizationVersion,
			KeyID:                "synthetic-key-id",
		},
		Key: key,
	}
}

func testValue(t *testing.T, kind contracts.Kind, payload string) contracts.Value {
	t.Helper()
	value, err := contracts.NewValue(kind, payload)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestNewStrategyAndNullHandling(t *testing.T) {
	ids := []string{"null", "blank", "redact", "format_random", "fake_email"}
	for _, id := range ids {
		ref := contracts.StrategyRef{ID: id, Version: strategyVersion}
		strategy, err := NewStrategy(ref)
		if err != nil || strategy.Ref() != ref {
			t.Fatalf("strategy %s unavailable", id)
		}
		got, err := strategy.Mask(context.Background(), contracts.Value{}, testMaskContext(t, id, contracts.Text))
		if err != nil || !got.IsNull() {
			t.Fatalf("strategy %s did not preserve NULL", id)
		}
	}
	for _, ref := range []contracts.StrategyRef{
		{ID: "unknown", Version: strategyVersion},
		{ID: "redact", Version: "v2"},
		{},
	} {
		if _, err := NewStrategy(ref); !errors.Is(err, contracts.ErrUnsupported) {
			t.Fatal("unsupported strategy accepted")
		}
	}

	strategy, _ := NewStrategy(contracts.StrategyRef{ID: "null", Version: strategyVersion})
	maskCtx := testMaskContext(t, "null", contracts.Text)
	input := testValue(t, contracts.Text, "synthetic")
	got, err := strategy.Mask(context.Background(), input, maskCtx)
	if err != nil || !got.IsNull() {
		t.Fatal("null strategy did not produce NULL")
	}
	maskCtx.Plan.Type.Nullable = false
	if _, err = strategy.Mask(context.Background(), input, maskCtx); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatal("null strategy ignored NOT NULL constraint")
	}
}

func TestBlankAndRedact(t *testing.T) {
	blank, _ := NewStrategy(contracts.StrategyRef{ID: "blank", Version: strategyVersion})
	for _, kind := range []contracts.Kind{contracts.Text, contracts.Bytes} {
		got, err := blank.Mask(context.Background(), testValue(t, kind, "synthetic"), testMaskContext(t, "blank", kind))
		if err != nil || got.Kind() != kind || got.Payload() != "" || got.IsNull() {
			t.Fatalf("blank strategy lost kind %d", kind)
		}
	}
	empty := testValue(t, contracts.Text, "")
	got, err := blank.Mask(context.Background(), empty, testMaskContext(t, "blank", contracts.Text))
	if err != nil || got != empty {
		t.Fatal("blank strategy rejected an already blank value")
	}
	if _, err = blank.Mask(context.Background(), testValue(t, contracts.Int, "1"), testMaskContext(t, "blank", contracts.Int)); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("blank strategy pretended to support integers")
	}

	redactStrategy, _ := NewStrategy(contracts.StrategyRef{ID: "redact", Version: strategyVersion})
	input := testValue(t, contracts.Text, "Alice-42@example.invalid")
	got, err = redactStrategy.Mask(context.Background(), input, testMaskContext(t, "redact", contracts.Text))
	if err != nil || got.Payload() != "*****-**@*******.*******" || got == input {
		t.Fatal("redact did not mask content while preserving separators")
	}
	binaryInput := testValue(t, contracts.Bytes, string([]byte{'A', '-', 0, '9'}))
	got, err = redactStrategy.Mask(context.Background(), binaryInput, testMaskContext(t, "redact", contracts.Bytes))
	if err != nil || got.Payload() != string([]byte{'*', '-', 0, '*'}) {
		t.Fatal("binary redaction changed separators or missed ASCII data")
	}
	if _, err = redactStrategy.Mask(context.Background(), testValue(t, contracts.Text, "***"), testMaskContext(t, "redact", contracts.Text)); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatal("unchanged non-empty redaction was accepted")
	}
}

func TestFormatRandomDeterministicAndTyped(t *testing.T) {
	strategy, _ := NewStrategy(contracts.StrategyRef{ID: "format_random", Version: strategyVersion})
	input := testValue(t, contracts.Text, "Abc-123-合成")
	maskCtx := testMaskContext(t, "format_random", contracts.Text)
	first, err := strategy.Mask(context.Background(), input, maskCtx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := strategy.Mask(context.Background(), input, maskCtx)
	if err != nil || first != second || first == input || !utf8.ValidString(first.Payload()) {
		t.Fatal("text format_random is not deterministic, changed, and valid UTF-8")
	}
	if first.Payload() != "Igj-458-海星" {
		t.Fatal("format_random golden output changed")
	}
	inputRunes, outputRunes := []rune(input.Payload()), []rune(first.Payload())
	if len(inputRunes) != len(outputRunes) {
		t.Fatal("text format_random changed rune count")
	}
	for i := range inputRunes {
		if unicode.IsLetter(inputRunes[i]) != unicode.IsLetter(outputRunes[i]) || unicode.IsDigit(inputRunes[i]) != unicode.IsDigit(outputRunes[i]) {
			t.Fatal("text format_random changed character class")
		}
	}

	binaryInput := testValue(t, contracts.Bytes, string([]byte{0, 1, 2, 255}))
	binaryOutput, err := strategy.Mask(context.Background(), binaryInput, testMaskContext(t, "format_random", contracts.Bytes))
	if err != nil || binaryOutput == binaryInput || len(binaryOutput.Payload()) != len(binaryInput.Payload()) {
		t.Fatal("binary format_random failed")
	}
	if _, err = strategy.Mask(context.Background(), testValue(t, contracts.Date, "2024-02-29"), testMaskContext(t, "format_random", contracts.Date)); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("format_random pretended to support Date")
	}
}

func TestFakeEmail(t *testing.T) {
	strategy, _ := NewStrategy(contracts.StrategyRef{ID: "fake_email", Version: strategyVersion})
	input := testValue(t, contracts.Text, "person@example.com")
	maskCtx := testMaskContext(t, "fake_email", contracts.Text)
	first, err := strategy.Mask(context.Background(), input, maskCtx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := strategy.Mask(context.Background(), input, maskCtx)
	if err != nil || first != second || first == input || !strings.HasSuffix(first.Payload(), "@example.invalid") {
		t.Fatal("fake_email output is not deterministic and safely non-deliverable")
	}
	if first.Payload() != "gw7ljvwp0cib@example.invalid" {
		t.Fatal("fake_email golden output changed")
	}
	if _, err = strategy.Mask(context.Background(), testValue(t, contracts.Text, "not-an-email"), maskCtx); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("fake_email accepted malformed input")
	}
	for _, malformed := range []string{".a@example.com", "a..b@example.com", "a@-example.com", "a@example..com"} {
		if _, err = strategy.Mask(context.Background(), testValue(t, contracts.Text, malformed), maskCtx); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatal("fake_email accepted malformed structure")
		}
	}
	if _, err = strategy.Mask(context.Background(), testValue(t, contracts.Bytes, "a@example.com"), testMaskContext(t, "fake_email", contracts.Bytes)); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("fake_email pretended to support bytes")
	}
	maskCtx.Plan.Type.Length = 5
	if _, err = strategy.Mask(context.Background(), input, maskCtx); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatal("fake_email ignored declared length")
	}
}

func TestMaskContextIsolationValidationAndCancellation(t *testing.T) {
	strategy, _ := NewStrategy(contracts.StrategyRef{ID: "format_random", Version: strategyVersion})
	input := testValue(t, contracts.Text, "SyntheticValue123456789")
	baseCtx := testMaskContext(t, "format_random", contracts.Text)
	baseInput, basePlan := input, baseCtx.Plan
	base, err := strategy.Mask(context.Background(), input, baseCtx)
	if err != nil {
		t.Fatal(err)
	}
	scopeCtx := baseCtx
	scopeCtx.Plan.Scope = "different-scope"
	byScope, _ := strategy.Mask(context.Background(), input, scopeCtx)
	keyCtx := baseCtx
	keyCtx.Key = testValue(t, contracts.Bytes, strings.Repeat("z", minimumKeyBytes))
	bySecret, _ := strategy.Mask(context.Background(), input, keyCtx)
	if base == byScope || base == bySecret {
		t.Fatal("scope or key change reused deterministic output")
	}
	if input != baseInput || baseCtx.Plan != basePlan {
		t.Fatal("Mask mutated its input DTO")
	}

	shortCtx := baseCtx
	shortCtx.Key = testValue(t, contracts.Bytes, "short")
	if _, err = strategy.Mask(context.Background(), input, shortCtx); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("short key accepted")
	}
	mismatchCtx := baseCtx
	mismatchCtx.Plan.Strategy.ID = "redact"
	if _, err = strategy.Mask(context.Background(), input, mismatchCtx); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("mismatched strategy plan accepted")
	}
	if _, err = strategy.Mask(nil, input, baseCtx); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = strategy.Mask(cancelled, input, baseCtx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not propagated")
	}
}

func TestMappingKeyIsolationAndLengthPrefixing(t *testing.T) {
	input := testValue(t, contracts.Text, "ab")
	baseCtx := testMaskContext(t, "format_random", contracts.Text)
	base, err := deriveMappingKey(context.Background(), input, baseCtx, baseCtx.Plan.Strategy)
	if err != nil {
		t.Fatal(err)
	}
	versionCtx := baseCtx
	versionCtx.Plan.Strategy.Version = "v2"
	byVersion, _ := deriveMappingKey(context.Background(), input, versionCtx, versionCtx.Plan.Strategy)
	bytesCtx := testMaskContext(t, "format_random", contracts.Bytes)
	byKind, _ := deriveMappingKey(context.Background(), testValue(t, contracts.Bytes, "ab"), bytesCtx, bytesCtx.Plan.Strategy)
	if base == byVersion || base == byKind {
		t.Fatal("version or typed identity reused a mapping key")
	}

	leftCtx := baseCtx
	leftCtx.Plan.Strategy = contracts.StrategyRef{ID: "ab", Version: "c"}
	left, _ := deriveMappingKey(context.Background(), input, leftCtx, leftCtx.Plan.Strategy)
	rightCtx := baseCtx
	rightCtx.Plan.Strategy = contracts.StrategyRef{ID: "a", Version: "bc"}
	right, _ := deriveMappingKey(context.Background(), input, rightCtx, rightCtx.Plan.Strategy)
	if left.Fingerprint == right.Fingerprint {
		t.Fatal("length-prefixed fields were concatenated ambiguously")
	}
}
