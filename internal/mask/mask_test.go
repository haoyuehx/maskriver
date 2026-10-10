package mask

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// newTextValue is a test helper that forces Value creation through the
// contracts.Value validation path. All test values go through this
// helper so that invalid representations fail fast.
func newTextValue(t *testing.T, s string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Text, s)
	if err != nil {
		t.Fatalf("NewValue(Text,%q) err=%v", trunc(s), err)
	}
	return v
}
func newBytesValue(t *testing.T, s string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Bytes, s)
	if err != nil {
		t.Fatalf("NewValue(Bytes,%q) err=%v", trunc(s), err)
	}
	return v
}
func newIntValue(t *testing.T, n int64) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(contracts.Int, fmt.Sprintf("%d", n))
	if err != nil {
		t.Fatalf("NewValue(Int,%d) err=%v", n, err)
	}
	return v
}

// testKey is the per-test 32B+ key material. Always Bytes and >=32B.
func testKey() contracts.Value {
	v, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("K", 64))
	return v
}

// shortKey is explicitly <32B and must always cause ErrInvalid.
func shortKey() contracts.Value {
	v, _ := contracts.NewValue(contracts.Bytes, "too-short")
	return v
}

// trunc keeps assertion messages short without leaking payloads into
// stdout when a test prints a value.
func trunc(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:16] + "…"
}

// basicPlan is the canonical per-test ColumnPlan used by strategy
// tests. It deliberately uses a short Scope so that it differs from
// planExtra() outputs used by adjacent tests.
func basicPlan() contracts.ColumnPlan {
	return contracts.ColumnPlan{
		Column:               contracts.ColumnRef{TableRef: contracts.TableRef{Database: "ds", Schema: "s", Table: "t"}, Column: "c"},
		Type:                 contracts.Type{Kind: contracts.Text, Native: "VARCHAR(64)"},
		Strategy:             contracts.StrategyRef{},
		Scope:                "tests/unit",
		NormalizationVersion: "m1a-v1",
		KeyID:                "unit-key-1",
	}
}

func Test_NewStrategy_RefRoundtrip(t *testing.T) {
	cases := []contracts.StrategyRef{
		refNull, refBlank, refRedact, refFormatRandom, refFakeEmail,
	}
	for _, ref := range cases {
		s, err := NewStrategy(ref)
		if err != nil {
			t.Fatalf("NewStrategy(%+v) err=%v", ref, err)
		}
		if s.Ref() != ref {
			t.Errorf("Ref mismatch: want %+v got %+v", ref, s.Ref())
		}
	}
}

func Test_NewStrategy_UnknownErrInvalid(t *testing.T) {
	_, err := NewStrategy(contracts.StrategyRef{ID: "unknown", Version: "1"})
	if err != contracts.ErrInvalid {
		t.Fatalf("unknown ref want ErrInvalid, got %v", err)
	}
	_, err = NewStrategy(contracts.StrategyRef{})
	if err != contracts.ErrInvalid {
		t.Fatalf("empty ref want ErrInvalid, got %v", err)
	}
}

func Test_Null_AlwaysNull(t *testing.T) {
	s, _ := NewStrategy(refNull)
	ctx := context.Background()
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	out, err := s.Mask(ctx, newTextValue(t, "alice@example.com"), mc)
	if err != nil {
		t.Fatalf("null mask err=%v", err)
	}
	if !out.IsNull() {
		t.Fatalf("null strategy must produce NULL, got %+v", out)
	}
	// Even with a null input, returns null.
	out, err = s.Mask(ctx, contracts.Value{}, mc)
	if err != nil || !out.IsNull() {
		t.Fatalf("null strategy on NULL want NULL, got err=%v val=%+v", err, out)
	}
}

func Test_Blank_NullAndEmpty(t *testing.T) {
	s, _ := NewStrategy(refBlank)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	ctx := context.Background()
	out, err := s.Mask(ctx, newTextValue(t, "anything"), mc)
	if err != nil {
		t.Fatalf("blank err=%v", err)
	}
	if out.Kind() != contracts.Text || out.Payload() != "" {
		t.Fatalf("blank text want empty-Text, got %+v", out)
	}
	out, err = s.Mask(ctx, contracts.Value{}, mc)
	if err != nil || !out.IsNull() {
		t.Fatalf("blank on NULL want NULL got %+v err=%v", out, err)
	}
}

func Test_Redact_Types(t *testing.T) {
	s, _ := NewStrategy(refRedact)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	ctx := context.Background()

	out, err := s.Mask(ctx, newTextValue(t, "something-secret"), mc)
	if err != nil || out.Payload() != "***REDACTED***" {
		t.Fatalf("redact text want ***REDACTED***, got %+v err=%v", out, err)
	}
	out, err = s.Mask(ctx, newBytesValue(t, "\xff\x01"), mc)
	if err != nil || out.Kind() != contracts.Bytes || out.Payload() != "" {
		t.Fatalf("redact bytes want empty-Bytes got %+v err=%v", out, err)
	}
	out, err = s.Mask(ctx, newIntValue(t, 1234), mc)
	if err != nil || out.Kind() != contracts.Int || out.Payload() != "0" {
		t.Fatalf("redact int want Int 0 got %+v err=%v", out, err)
	}
	out, err = s.Mask(ctx, contracts.Value{}, mc)
	if err != nil || !out.IsNull() {
		t.Fatalf("redact on NULL want NULL got %+v err=%v", out, err)
	}
}

func Test_FormatRandom_UnsupportedKind(t *testing.T) {
	s, _ := NewStrategy(refFormatRandom)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	ctx := context.Background()
	if _, err := s.Mask(ctx, newIntValue(t, 42), mc); err != contracts.ErrUnsupported {
		t.Fatalf("format_random on Int want ErrUnsupported, got %v", err)
	}
}

func Test_FormatRandom_ShortKey_Invalid(t *testing.T) {
	s, _ := NewStrategy(refFormatRandom)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: shortKey()}
	ctx := context.Background()
	if _, err := s.Mask(ctx, newTextValue(t, "abc"), mc); err != contracts.ErrInvalid {
		t.Fatalf("format_random short key want ErrInvalid got %v", err)
	}
}

func Test_FormatRandom_Deterministic(t *testing.T) {
	s, _ := NewStrategy(refFormatRandom)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	ctx := context.Background()
	val := newTextValue(t, "user@example.com")
	a, err := s.Mask(ctx, val, mc)
	if err != nil {
		t.Fatalf("mask err=%v", err)
	}
	b, err := s.Mask(ctx, val, mc)
	if err != nil {
		t.Fatalf("mask2 err=%v", err)
	}
	if a != b {
		t.Fatalf("non-deterministic: %+v vs %+v", a, b)
	}
	if a.Payload() == val.Payload() {
		t.Fatalf("format_random did not transform payload")
	}
}

func Test_FormatRandom_ScopeIsolation(t *testing.T) {
	s, _ := NewStrategy(refFormatRandom)
	ctx := context.Background()
	k := testKey()
	val := newTextValue(t, "same-input")
	pa := basicPlan()
	pb := basicPlan()
	pb.Scope = "tests/other"
	a, err := s.Mask(ctx, val, contracts.MaskContext{Plan: pa, Key: k})
	if err != nil {
		t.Fatalf("maskA err=%v", err)
	}
	b, err := s.Mask(ctx, val, contracts.MaskContext{Plan: pb, Key: k})
	if err != nil {
		t.Fatalf("maskB err=%v", err)
	}
	if a == b {
		t.Fatalf("different scopes produced identical output: %+v", a)
	}
}

func Test_FormatRandom_KeyIDIsolation(t *testing.T) {
	s, _ := NewStrategy(refFormatRandom)
	ctx := context.Background()
	val := newTextValue(t, "same-input")
	pa := basicPlan()
	pb := basicPlan()
	pb.KeyID = "other-key"
	a, _ := s.Mask(ctx, val, contracts.MaskContext{Plan: pa, Key: testKey()})
	b, _ := s.Mask(ctx, val, contracts.MaskContext{Plan: pb, Key: testKey()})
	if a == b {
		t.Fatalf("different keyID produced identical output")
	}
}

func Test_FakeEmail_ShapeAndDeterminism(t *testing.T) {
	s, _ := NewStrategy(refFakeEmail)
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	ctx := context.Background()
	val := newTextValue(t, "alice+tag@domain.co.uk")
	out, err := s.Mask(ctx, val, mc)
	if err != nil {
		t.Fatalf("fake_email err=%v", err)
	}
	if out.Kind() != contracts.Text {
		t.Fatalf("fake_email Kind want Text got %v", out.Kind())
	}
	p := out.Payload()
	if !strings.HasSuffix(p, "@"+maskDomain) {
		t.Fatalf("fake_email want @%s suffix, got %q", maskDomain, trunc(p))
	}
	out2, err := s.Mask(ctx, val, mc)
	if err != nil {
		t.Fatalf("fake_email2 err=%v", err)
	}
	if out != out2 {
		t.Fatalf("fake_email non-deterministic: %q vs %q", trunc(out.Payload()), trunc(out2.Payload()))
	}
}

func Test_FakeEmail_KindsAndKeys(t *testing.T) {
	s, _ := NewStrategy(refFakeEmail)
	ctx := context.Background()
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	if _, err := s.Mask(ctx, newBytesValue(t, "not-text"), mc); err != contracts.ErrUnsupported {
		t.Fatalf("fake_email on Bytes want ErrUnsupported got %v", err)
	}
	badKey := contracts.MaskContext{Plan: basicPlan(), Key: shortKey()}
	if _, err := s.Mask(ctx, newTextValue(t, "x"), badKey); err != contracts.ErrInvalid {
		t.Fatalf("fake_email short key want ErrInvalid got %v", err)
	}
	// NULL passes through unchanged.
	out, err := s.Mask(ctx, contracts.Value{}, mc)
	if err != nil || !out.IsNull() {
		t.Fatalf("fake_email NULL want NULL got %+v err=%v", out, err)
	}
}

func Test_MemoryMapping_LookupAndGetOrCreate(t *testing.T) {
	m := NewMemoryMapping()
	ctx := context.Background()
	k := contracts.MappingKey{
		Strategy:             refNull,
		Scope:                "s",
		NormalizationVersion: "1",
		KeyID:                "k",
		Fingerprint:          [32]byte{1, 2, 3},
	}
	if _, ok, err := m.Lookup(ctx, k); err != nil || ok {
		t.Fatalf("fresh lookup should miss, got ok=%v err=%v", ok, err)
	}
	a := newTextValue(t, "winner-a")
	got, err := m.GetOrCreate(ctx, k, a)
	if err != nil || got != a {
		t.Fatalf("GetOrCreate first call want a got %+v err=%v", got, err)
	}
	b := newTextValue(t, "loser-b")
	got, err = m.GetOrCreate(ctx, k, b)
	if err != nil || got != a {
		t.Fatalf("GetOrCreate second call want existing a got %+v err=%v", got, err)
	}
	found, ok, err := m.Lookup(ctx, k)
	if err != nil || !ok || found != a {
		t.Fatalf("Lookup after write want a ok=true, got %+v ok=%v err=%v", found, ok, err)
	}
	if m.Len() != 1 {
		t.Fatalf("Len want 1 got %d", m.Len())
	}
}

func Test_MemoryMapping_ConcurrentSingleWinner(t *testing.T) {
	m := NewMemoryMapping()
	ctx := context.Background()
	const N = 128
	k := contracts.MappingKey{
		Strategy:             refFormatRandom,
		Scope:                "concurrency",
		NormalizationVersion: "1",
		KeyID:                "k1",
		Fingerprint:          [32]byte{0xFF},
	}
	values := make([]contracts.Value, N)
	for i := 0; i < N; i++ {
		values[i] = newTextValue(t, fmt.Sprintf("cand-%04x", i))
	}
	winners := make([]contracts.Value, N)
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			winners[idx], errs[idx] = m.GetOrCreate(ctx, k, values[idx])
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("goroutine %d err=%v", i, errs[i])
		}
	}
	canonical := winners[0]
	for i := 1; i < N; i++ {
		if winners[i] != canonical {
			t.Fatalf("non-unanimous winner at %d: %+v vs %+v", i, winners[i], canonical)
		}
	}
	if m.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", m.Len())
	}
}

func Test_MemoryMapping_ContextCanceled_BeforeLock(t *testing.T) {
	m := NewMemoryMapping()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	k := contracts.MappingKey{Fingerprint: [32]byte{0xAB}}
	if _, err := m.GetOrCreate(ctx, k, newTextValue(t, "any")); err != context.Canceled {
		t.Fatalf("canceled context want context.Canceled got %v", err)
	}
	if m.Len() != 0 {
		t.Fatalf("canceled context should not insert entry, len=%d", m.Len())
	}
}

func Test_MemoryMapping_NilStore(t *testing.T) {
	var m *MemoryMapping
	ctx := context.Background()
	if _, ok, err := m.Lookup(ctx, contracts.MappingKey{}); err != nil || ok {
		t.Fatalf("nil lookup should miss cleanly ok=%v err=%v", ok, err)
	}
	if _, err := m.GetOrCreate(ctx, contracts.MappingKey{}, newTextValue(t, "x")); err != contracts.ErrClosed {
		t.Fatalf("nil GetOrCreate want ErrClosed got %v", err)
	}
}
