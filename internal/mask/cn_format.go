package mask

import (
	"context"
	"encoding/binary"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// cnFormatStrategy implements bounded, deterministic format-constrained masking.
// It is NOT format-preserving encryption, a uniqueness guarantee, or a proof
// that a generated number is unassigned. Apply only to authorized test data.
type cnFormatStrategy struct{ ref contracts.StrategyRef }

const cnFormatVersion = "1"

var _ contracts.Strategy = (*cnFormatStrategy)(nil)

func newCNFormatStrategy(ref contracts.StrategyRef) (contracts.Strategy, error) {
	if ref.Version != cnFormatVersion {
		return nil, contracts.ErrInvalid
	}
	switch ref.ID {
	case "cn_id_card", "cn_mobile", "cn_bank_card", "cn_uscc",
		"cn_postal_code", "cn_city", "cn_person_name", "cn_address", "cn_date":
		return &cnFormatStrategy{ref: ref}, nil
	default:
		return nil, contracts.ErrInvalid
	}
}

func (s *cnFormatStrategy) Ref() contracts.StrategyRef { return s.ref }

func (s *cnFormatStrategy) Mask(ctx context.Context, input contracts.Value, mc contracts.MaskContext) (contracts.Value, error) {
	if err := checkMaskContext(ctx); err != nil {
		return contracts.Value{}, err
	}
	if err := validateKey(mc.Key); err != nil {
		return contracts.Value{}, err
	}
	if mc.Plan.Strategy != s.ref || mc.Plan.Scope == "" ||
		mc.Plan.NormalizationVersion == "" || mc.Plan.KeyID == "" {
		return contracts.Value{}, contracts.ErrInvalid
	}
	if input.IsNull() {
		return input, nil
	}
	if mc.Plan.Type.Kind != input.Kind() {
		return contracts.Value{}, contracts.ErrInvalid
	}
	if input.Kind() != contracts.Text && !(s.ref.ID == "cn_date" && input.Kind() == contracts.Date) {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	seed := typedLexicalDigest(input, mc.Key, planExtra(mc))
	var output string
	var err error
	source := input.Payload()
	switch s.ref.ID {
	case "cn_id_card":
		output, err = replaceCNID(source, seed)
	case "cn_mobile":
		output, err = replaceCNMobile(source, seed)
	case "cn_bank_card":
		output, err = replaceCNBank(source, seed)
	case "cn_uscc":
		output, err = replaceCNUSCC(source, seed)
	case "cn_postal_code":
		output, err = replaceCNPostal(source, seed)
	case "cn_city":
		output, err = replaceCNCity(source, seed)
	case "cn_person_name":
		output, err = replaceCNName(source, seed)
	case "cn_address":
		output, err = replaceCNAddress(source, seed)
	case "cn_date":
		output, err = replaceCNDate(source, seed, input.Kind())
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err != nil {
		return contracts.Value{}, err
	}
	if output == source {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	if mc.Plan.Type.Length > 0 && utf8.RuneCountInString(output) > mc.Plan.Type.Length {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return contracts.NewValue(input.Kind(), output)
}

// cnNumber is derived from HMAC input bound to input Kind, strategy,
// normalization version, scope and key ID. No source material is stored.
func cnNumber(seed [32]byte, label string, limit uint64) uint64 {
	var n [8]byte
	block := hmacSHA256(seed[:], []byte(label))
	copy(n[:], block[:8])
	return binary.BigEndian.Uint64(n[:]) % limit
}

func cnDigits(seed [32]byte, label string, length int) string {
	var b strings.Builder
	b.Grow(length)
	for i := 0; i < length; i++ {
		b.WriteByte(byte('0' + cnNumber(seed, label+"_"+cnDecimal(i), 10)))
	}
	return b.String()
}

func cnDecimal(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

func cnASCII(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func cnDateValid(s string, layout string) bool {
	t, err := time.Parse(layout, s)
	return err == nil && t.Format(layout) == s && t.Year() >= 1900
}

func cnHan(s string, low, high int) bool {
	runes := []rune(s)
	if len(runes) < low || len(runes) > high {
		return false
	}
	for _, r := range runes {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return true
}

// A deliberately narrow, public structural form: a Han place/road stem,
// street marker, ASCII house number and 号. It is not a street dictionary.
var cnAddressShape = regexp.MustCompile("^[\\p{Han}]{2,32}(路|街|大道|巷)[0-9]{1,4}号$")
