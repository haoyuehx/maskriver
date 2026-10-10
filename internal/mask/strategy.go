package mask

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
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
	if k.Kind() != contracts.Bytes && k.Kind() != contracts.Text {
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

// ----- individual strategies -----------------------------------------

type nullStrategy struct{ ref contracts.StrategyRef }

func (s *nullStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *nullStrategy) Mask(_ context.Context, _ contracts.Value, _ contracts.MaskContext) (contracts.Value, error) {
	// Always returns SQL NULL for any input. Key is deliberately not
	// consulted; null is a structural (non-sensitive) mapping.
	return contracts.Value{}, nil
}

type blankStrategy struct{ ref contracts.StrategyRef }

func (s *blankStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *blankStrategy) Mask(_ context.Context, val contracts.Value, _ contracts.MaskContext) (contracts.Value, error) {
	// Null remains null; everything else becomes empty-Text.
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	return contracts.NewValue(contracts.Text, "")
}

type redactStrategy struct{ ref contracts.StrategyRef }

func (s *redactStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *redactStrategy) Mask(_ context.Context, val contracts.Value, _ contracts.MaskContext) (contracts.Value, error) {
	// Redact replaces textual payloads with "***REDACTED***". Non-text
	// non-null inputs retain their Kind but the payload becomes an
	// intentionally useless zero-like value (empty) to discourage
	// leakage through downstream type coercion.
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	switch val.Kind() {
	case contracts.Text:
		return contracts.NewValue(contracts.Text, "***REDACTED***")
	case contracts.Bytes:
		return contracts.NewValue(contracts.Bytes, "")
	case contracts.Int:
		return contracts.NewValue(contracts.Int, "0")
	case contracts.Uint:
		return contracts.NewValue(contracts.Uint, "0")
	case contracts.Float:
		return contracts.NewValue(contracts.Float, "0")
	case contracts.Decimal:
		return contracts.NewValue(contracts.Decimal, "0")
	case contracts.Bool:
		return contracts.NewValue(contracts.Bool, "false")
	case contracts.Date, contracts.LocalDateTime, contracts.Instant:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	return contracts.Value{}, contracts.ErrUnsupported
}

// formatRandomStrategy produces deterministic "random"-looking values
// derived purely from HMAC. It only supports Text and Bytes; every
// other Kind returns ErrUnsupported (format_random does not invent
// plausible numeric output, as that would require a numeric-preserving
// strategy which Main has not authorized in M1-B).
type formatRandomStrategy struct{ ref contracts.StrategyRef }

func (s *formatRandomStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *formatRandomStrategy) Mask(_ context.Context, val contracts.Value, ctx contracts.MaskContext) (contracts.Value, error) {
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	switch val.Kind() {
	case contracts.Text, contracts.Bytes:
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err := validateKey(ctx.Key); err != nil {
		return contracts.Value{}, err
	}
	digest := typedLexicalDigest(val, ctx.Key, planExtra(ctx))
	hexed := hex.EncodeToString(digest[:])
	// Trim / grow output to the same UTF-8 length as the original
	// textual payload when possible; Text stays at or under 64 bytes
	// to keep tests stable.
	wantLen := len(val.Payload())
	if wantLen <= 0 {
		wantLen = 16
	}
	if wantLen > 64 {
		wantLen = 64
	}
	out := hexed
	for len(out) < wantLen {
		// Double-hash to extend without introducing RNG.
		next := hmacSHA256([]byte(ctx.Key.Payload()), []byte(hexed+maskDomain))
		out += hex.EncodeToString(next[:])
	}
	out = out[:wantLen]
	if val.Kind() == contracts.Bytes {
		return contracts.NewValue(contracts.Bytes, out)
	}
	return contracts.NewValue(contracts.Text, out)
}

// fakeEmailStrategy produces deterministic <hex-16>@maskriver.local
// email-like tokens. The local part is derived from the HMAC digest
// so that the same input always maps to the same fake address.
type fakeEmailStrategy struct{ ref contracts.StrategyRef }

func (s *fakeEmailStrategy) Ref() contracts.StrategyRef { return s.ref }
func (s *fakeEmailStrategy) Mask(_ context.Context, val contracts.Value, ctx contracts.MaskContext) (contracts.Value, error) {
	if val.IsNull() {
		return contracts.Value{}, nil
	}
	if val.Kind() != contracts.Text {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err := validateKey(ctx.Key); err != nil {
		return contracts.Value{}, err
	}
	digest := typedLexicalDigest(val, ctx.Key, planExtra(ctx))
	local := hex.EncodeToString(digest[:16])
	// Append a tiny CRC tag bound to the original length so that long
	// and short emails with identical prefixes do not collide at the
	// output when truncated by the caller.
	crc := crc32.ChecksumIEEE([]byte(val.Payload()))
	local = fmt.Sprintf("%s%08x", local, crc)
	if len(local) > 40 {
		local = local[:40]
	}
	return contracts.NewValue(contracts.Text, local+"@"+maskDomain)
}
