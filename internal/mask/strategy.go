package mask

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	strategyVersion      = "v1"
	normalizationVersion = "typed-lexical-v1"
	minimumKeyBytes      = 32
)

var _ contracts.Strategy = (*strategy)(nil)

type strategy struct {
	ref contracts.StrategyRef
}

// NewStrategy constructs one of the bounded M1 masking strategies.
func NewStrategy(ref contracts.StrategyRef) (contracts.Strategy, error) {
	if ref.Version != strategyVersion {
		return nil, contracts.ErrUnsupported
	}
	switch ref.ID {
	case "null", "blank", "redact", "format_random", "fake_email":
		return &strategy{ref: ref}, nil
	default:
		return nil, contracts.ErrUnsupported
	}
}

func (s *strategy) Ref() contracts.StrategyRef { return s.ref }

func (s *strategy) Mask(ctx context.Context, input contracts.Value, maskCtx contracts.MaskContext) (contracts.Value, error) {
	key, err := deriveMappingKey(ctx, input, maskCtx, s.ref)
	if err != nil {
		return contracts.Value{}, err
	}
	if input.IsNull() {
		return input, nil
	}

	var output contracts.Value
	switch s.ref.ID {
	case "null":
		if !maskCtx.Plan.Type.Nullable {
			return contracts.Value{}, contracts.ErrUnsafe
		}
		output = contracts.Value{}
	case "blank":
		if input.Kind() != contracts.Text && input.Kind() != contracts.Bytes {
			return contracts.Value{}, contracts.ErrUnsupported
		}
		output, err = contracts.NewValue(input.Kind(), "")
	case "redact":
		output, err = redact(input)
	case "format_random":
		output, err = formatRandom(input, key.Fingerprint)
	case "fake_email":
		output, err = fakeEmail(input, key.Fingerprint, maskCtx.Plan.Type.Length)
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if err != nil {
		return contracts.Value{}, err
	}
	if input.Payload() != "" && output == input {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return output, nil
}

func deriveMappingKey(ctx context.Context, input contracts.Value, maskCtx contracts.MaskContext, ref contracts.StrategyRef) (contracts.MappingKey, error) {
	if ctx == nil {
		return contracts.MappingKey{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return contracts.MappingKey{}, err
	}
	plan := maskCtx.Plan
	if maskCtx.Key.Kind() != contracts.Bytes || len(maskCtx.Key.Payload()) < minimumKeyBytes ||
		ref.ID == "" || ref.Version == "" || plan.Strategy != ref || plan.Scope == "" ||
		plan.NormalizationVersion != normalizationVersion || plan.KeyID == "" {
		return contracts.MappingKey{}, contracts.ErrInvalid
	}
	if !input.IsNull() && plan.Type.Kind != input.Kind() {
		return contracts.MappingKey{}, contracts.ErrInvalid
	}

	mac := hmac.New(sha256.New, []byte(maskCtx.Key.Payload()))
	writeField(mac, []byte{byte(input.Kind())})
	writeField(mac, []byte(input.Payload()))
	writeField(mac, []byte(ref.ID))
	writeField(mac, []byte(ref.Version))
	writeField(mac, []byte(plan.Scope))
	writeField(mac, []byte(plan.NormalizationVersion))
	writeField(mac, []byte(plan.KeyID))
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], mac.Sum(nil))
	return contracts.MappingKey{
		Strategy:             ref,
		Scope:                plan.Scope,
		NormalizationVersion: plan.NormalizationVersion,
		KeyID:                plan.KeyID,
		Fingerprint:          fingerprint,
	}, nil
}

type fieldWriter interface {
	Write([]byte) (int, error)
}

func writeField(w fieldWriter, field []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(field)))
	_, _ = w.Write(length[:])
	_, _ = w.Write(field)
}

func redact(input contracts.Value) (contracts.Value, error) {
	switch input.Kind() {
	case contracts.Text:
		var b strings.Builder
		for _, r := range input.Payload() {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteByte('*')
			} else {
				b.WriteRune(r)
			}
		}
		return contracts.NewValue(contracts.Text, b.String())
	case contracts.Bytes:
		masked := []byte(input.Payload())
		for i, value := range masked {
			if isASCIIAlphaNumeric(value) {
				masked[i] = '*'
			}
		}
		return contracts.NewValue(contracts.Bytes, string(masked))
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
}

func formatRandom(input contracts.Value, fingerprint [sha256.Size]byte) (contracts.Value, error) {
	switch input.Kind() {
	case contracts.Text:
		random := expand(fingerprint, utf8.RuneCountInString(input.Payload()))
		var b strings.Builder
		mutable := false
		randomIndex := 0
		for _, r := range input.Payload() {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune('a' + rune(random[randomIndex]%26))
				mutable = true
			case r >= 'A' && r <= 'Z':
				b.WriteRune('A' + rune(random[randomIndex]%26))
				mutable = true
			case r >= '0' && r <= '9':
				b.WriteRune('0' + rune(random[randomIndex]%10))
				mutable = true
			case unicode.IsLetter(r):
				alphabet := []rune("山河川林海星云月")
				b.WriteRune(alphabet[int(random[randomIndex])%len(alphabet)])
				mutable = true
			case unicode.IsDigit(r):
				b.WriteRune('0' + rune(random[randomIndex]%10))
				mutable = true
			default:
				b.WriteRune(r)
			}
			randomIndex++
		}
		if !mutable {
			return contracts.Value{}, contracts.ErrUnsupported
		}
		result := b.String()
		if result == input.Payload() {
			result = forceDifferentText(result)
		}
		return contracts.NewValue(contracts.Text, result)
	case contracts.Bytes:
		if input.Payload() == "" {
			return contracts.NewValue(contracts.Bytes, "")
		}
		result := expand(fingerprint, len(input.Payload()))
		if string(result) == input.Payload() {
			result[0] ^= 0xff
		}
		return contracts.NewValue(contracts.Bytes, string(result))
	default:
		return contracts.Value{}, contracts.ErrUnsupported
	}
}

func forceDifferentText(value string) string {
	runes := []rune(value)
	for i, r := range runes {
		switch {
		case r >= 'a' && r <= 'z':
			runes[i] = 'a' + (r-'a'+1)%26
		case r >= 'A' && r <= 'Z':
			runes[i] = 'A' + (r-'A'+1)%26
		case r >= '0' && r <= '9':
			runes[i] = '0' + (r-'0'+1)%10
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			runes[i] = '山'
		default:
			continue
		}
		if runes[i] == r {
			runes[i] = '河'
		}
		return string(runes)
	}
	return value
}

func fakeEmail(input contracts.Value, fingerprint [sha256.Size]byte, maximumLength int) (contracts.Value, error) {
	if input.Kind() != contracts.Text {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	if !validEmail(input.Payload()) {
		return contracts.Value{}, contracts.ErrInvalid
	}
	const domain = "@example.invalid"
	localLength := 12
	if maximumLength > 0 && localLength+len(domain) > maximumLength {
		localLength = maximumLength - len(domain)
	}
	if localLength < 1 {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	alphabet := "abcdefghijklmnopqrstuvwxyz0123456789"
	random := expand(fingerprint, localLength)
	local := make([]byte, localLength)
	for i := range local {
		local[i] = alphabet[int(random[i])%len(alphabet)]
	}
	result := string(local) + domain
	if result == input.Payload() {
		local[0] = alphabet[(strings.IndexByte(alphabet, local[0])+1)%len(alphabet)]
		result = string(local) + domain
	}
	return contracts.NewValue(contracts.Text, result)
}

func validEmail(value string) bool {
	if value == "" || len(value) > 254 || strings.TrimSpace(value) != value || strings.Count(value, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(value, "@")
	if local == "" || len(local) > 64 || domain == "" || len(domain) > 253 ||
		strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}
	for _, value := range []byte(local) {
		if !isASCIIAlphaNumeric(value) && !strings.ContainsRune(".!#$%&'*+/=?^_`{|}~-", rune(value)) {
			return false
		}
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, value := range []byte(label) {
			if !isASCIIAlphaNumeric(value) && value != '-' {
				return false
			}
		}
	}
	return true
}

func expand(fingerprint [sha256.Size]byte, size int) []byte {
	result := make([]byte, 0, size)
	for counter := uint64(0); len(result) < size; counter++ {
		mac := hmac.New(sha256.New, fingerprint[:])
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], counter)
		_, _ = mac.Write(encoded[:])
		result = append(result, mac.Sum(nil)...)
	}
	return result[:size]
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
