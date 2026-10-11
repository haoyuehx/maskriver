package mask

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Strategy registry. Refs use the exact identifiers expected by the
// Main-owned plan publishing rules. Unknown refs return ErrInvalid.
var (
	refNull         = contracts.StrategyRef{ID: "null", Version: "1"}
	refBlank        = contracts.StrategyRef{ID: "blank", Version: "1"}
	refRedact       = contracts.StrategyRef{ID: "redact", Version: "1"}
	refFormatRandom = contracts.StrategyRef{ID: "format_random", Version: "1"}
	refFakeEmail    = contracts.StrategyRef{ID: "fake_email", Version: "1"}
)

// NewStrategy returns a strategy by ref. Unknown refs or nil refs are
// reported as ErrInvalid; no I/O is performed.
func NewStrategy(ref contracts.StrategyRef) (contracts.Strategy, error) {
	switch ref {
	case contracts.StrategyRef{}:
		return nil, contracts.ErrInvalid
	case refNull:
		return &nullStrategy{ref: refNull}, nil
	case refBlank:
		return &blankStrategy{ref: refBlank}, nil
	case refRedact:
		return &redactStrategy{ref: refRedact}, nil
	case refFormatRandom:
		return &formatRandomStrategy{ref: refFormatRandom}, nil
	case refFakeEmail:
		return &fakeEmailStrategy{ref: refFakeEmail}, nil
	default:
		return nil, contracts.ErrInvalid
	}
}

// ----- helpers --------------------------------------------------------

// maskDomain is the opaque suffix appended to all format_random and
// fake_email outputs; it is fixed and never includes real domains.
const maskDomain = "maskriver.local"

// validateKey asserts that the caller-provided key is Bytes with at
// least 32 bytes of material. All deterministic strategies must call
// validateKey before producing output.
func validateKey(k contracts.Value) error {
	if k.Kind() != contracts.Bytes {
		return contracts.ErrInvalid
	}
	if len(k.Payload()) < 32 {
		return contracts.ErrInvalid
	}
	return nil
}

// typedLexicalDigest produces a 32-byte HMAC-SHA256 whose input
// strictly binds Kind and Value payload length to prevent cross-kind
// collisions. The extra blob binds scope/version/keyID so that the
// same original value masks differently across plans.
func typedLexicalDigest(val contracts.Value, k contracts.Value, extra string) [32]byte {
	// Input layout:
	//   kind(1B) | len_payload(4B BE) | payload_bytes | len_extra(4B BE) | extra_bytes
	// | marker "typed-lexical-v1"
	const marker = "typed-lexical-v1"
	payload := val.Payload()
	buf := make([]byte, 0, 1+4+len(payload)+4+len(extra)+len(marker))
	buf = append(buf, byte(val.Kind()))
	var len4 [4]byte
	binary.BigEndian.PutUint32(len4[:], uint32(len(payload)))
	buf = append(buf, len4[:]...)
	buf = append(buf, []byte(payload)...)
	binary.BigEndian.PutUint32(len4[:], uint32(len(extra)))
	buf = append(buf, len4[:]...)
	buf = append(buf, []byte(extra)...)
	buf = append(buf, []byte(marker)...)
	return hmacSHA256([]byte(k.Payload()), buf)
}

// hashExtra composes the plan-level extra input used by every
// strategy. It intentionally uses StrategyRef+Scope+NormVersion+KeyID
// in a single deterministic concatenation with length-prefixed fields
// so that partial colliding values cannot collide when hashed.
func planExtra(ctx contracts.MaskContext) string {
	ref := ctx.Plan.Strategy
	parts := []string{
		fmt.Sprintf("%d:%s", len(ref.ID), ref.ID),
		fmt.Sprintf("%d:%s", len(ref.Version), ref.Version),
		fmt.Sprintf("%d:%s", len(ctx.Plan.Scope), ctx.Plan.Scope),
		fmt.Sprintf("%d:%s", len(ctx.Plan.NormalizationVersion), ctx.Plan.NormalizationVersion),
		fmt.Sprintf("%d:%s", len(ctx.Plan.KeyID), ctx.Plan.KeyID),
	}
	return strings.Join(parts, "|")
}

func checkMaskContext(ctx context.Context) error {
	if ctx == nil {
		return contracts.ErrInvalid
	}
	return ctx.Err()
}

func strategyExtra(ctx contracts.MaskContext, ref contracts.StrategyRef) (string, error) {
	if ctx.Plan.Strategy != (contracts.StrategyRef{}) && ctx.Plan.Strategy != ref {
		return "", contracts.ErrInvalid
	}
	ctx.Plan.Strategy = ref
	return planExtra(ctx), nil
}

// ----- individual strategies -----------------------------------------

type nullStrategy struct{ ref contracts.StrategyRef }

func (s *nullStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *nullStrategy) Mask(ctx context.Context, val contracts.Value, mc contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if !val.IsNull() && !mc.Plan.Type.Nullable {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return contracts.Value{}, nil
}

type blankStrategy struct{ ref contracts.StrategyRef }

func (s *blankStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *blankStrategy) Mask(ctx context.Context, val contracts.Value, _ contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	if val.Kind() != contracts.Text && val.Kind() != contracts.Bytes {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	return contracts.NewValue(val.Kind(), "")
}

type redactStrategy struct{ ref contracts.StrategyRef }

func (s *redactStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *redactStrategy) Mask(ctx context.Context, val contracts.Value, _ contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	var raw string
	switch val.Kind() {
	case contracts.Text:
		raw = "***REDACTED***"
	case contracts.Bytes:
		raw = ""
	case contracts.Int, contracts.Uint, contracts.Float, contracts.Decimal:
		raw = "0"
	case contracts.Bool:
		raw = "false"
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if val.Payload() != "" && raw == val.Payload() {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return contracts.NewValue(val.Kind(), raw)
}

// formatRandomStrategy produces deterministic "random"-looking values
// derived purely from HMAC. It only supports Text and Bytes; every
// other Kind returns ErrUnsupported (format_random does not invent
// plausible numeric output, as that would require a numeric-preserving
// strategy which Main has not authorized in M1-B).
type formatRandomStrategy struct{ ref contracts.StrategyRef }

func (s *formatRandomStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *formatRandomStrategy) Mask(ctx context.Context, val contracts.Value, mc contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	if val.Kind() != contracts.Text && val.Kind() != contracts.Bytes {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err := validateKey(mc.Key); err != nil {
		return contracts.Value{}, err
	}
	extra, err := strategyExtra(mc, s.ref)
	if err != nil {
		return contracts.Value{}, err
	}
	return deterministicFormatRandom(val, mc.Key, extra)
}

// fakeEmailStrategy produces deterministic <hex-16>@maskriver.local
// email-like tokens. The local part is derived from the HMAC digest
// so that the same input always maps to the same fake address.
type fakeEmailStrategy struct{ ref contracts.StrategyRef }

func (s *fakeEmailStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *fakeEmailStrategy) Mask(ctx context.Context, val contracts.Value, mc contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	if val.Kind() != contracts.Text {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err := validateKey(mc.Key); err != nil {
		return contracts.Value{}, err
	}
	extra, err := strategyExtra(mc, s.ref)
	if err != nil {
		return contracts.Value{}, err
	}
	digest := typedLexicalDigest(val, mc.Key, extra)
	// Output must never contain an unkeyed digest of the original input.
	local := hex.EncodeToString(digest[:16])
	out := local + "@" + maskDomain
	if val.Payload() != "" && out == val.Payload() {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return contracts.NewValue(contracts.Text, out)
}
