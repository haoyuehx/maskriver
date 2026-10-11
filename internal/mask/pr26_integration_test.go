package mask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestPR26FakeEmailRejectsInvalidInput(t *testing.T) {
	strategy, err := NewStrategy(refFakeEmail)
	if err != nil {
		t.Fatal(err)
	}
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	for _, input := range []string{
		"not-an-email",
		".name@example.com",
		"hello..world@example.com",
		"hello@-example.com",
		"hello@example..com",
		"hello@localhost",
		"é@example.com",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := strategy.Mask(context.Background(), newTextValue(t, input), mc); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("invalid email was accepted: %v", err)
			}
		})
	}
}

func TestPR26FakeEmailRejectsTooNarrowColumn(t *testing.T) {
	strategy, err := NewStrategy(refFakeEmail)
	if err != nil {
		t.Fatal(err)
	}
	value := newTextValue(t, "synthetic@example.com")
	mc := contracts.MaskContext{Plan: basicPlan(), Key: testKey()}
	mc.Plan.Type.Length = 8
	if _, err := strategy.Mask(context.Background(), value, mc); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("too-small target length must fail closed: %v", err)
	}
	mc.Plan.Type.Length = 128
	output, err := strategy.Mask(context.Background(), value, mc)
	if err != nil || !strings.HasSuffix(output.Payload(), "@"+maskDomain) {
		t.Fatalf("valid email masking should still work: %v", err)
	}
}

func TestPR26MemoryMappingFormattingRedacted(t *testing.T) {
	m := NewMemoryMapping()
	key := contracts.MappingKey{Strategy: refFakeEmail, Scope: "synthetic", Fingerprint: [32]byte{1}}
	value := newTextValue(t, "synthetic-masked-secret")
	if _, err := m.GetOrCreate(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(m),
		fmt.Sprintf("%v", m),
		fmt.Sprintf("%+v", m),
		fmt.Sprintf("%#v", m),
	} {
		if rendered != "[REDACTED]" {
			t.Fatal("memory mapping exposes internal entries")
		}
	}
}
