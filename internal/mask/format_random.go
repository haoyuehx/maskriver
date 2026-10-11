package mask

import (
	"encoding/binary"
	"unicode"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This is deterministic masking, NOT cryptographic format-preserving encryption.
// Text keeps supported character classes and rune count. Bytes keeps byte count.
// Unsupported scripts and values that cannot change safely fail closed.
func deterministicFormatRandom(val, key contracts.Value, extra string) (contracts.Value, error) {
	seed := typedLexicalDigest(val, key, extra)
	source := val.Payload()
	if len(source) == 0 {
		return val, nil
	}
	if val.Kind() == contracts.Bytes {
		dst := make([]byte, len(source))
		for i := range dst {
			var counter [8]byte
			binary.BigEndian.PutUint64(counter[:], uint64(i/32))
			message := append(append(make([]byte, 0, 40), seed[:]...), counter[:]...)
			block := hmacSHA256([]byte(key.Payload()), message)
			dst[i] = block[i%32]
		}
		if string(dst) == source {
			dst[0] ^= 1
		}
		return contracts.NewValue(contracts.Bytes, string(dst))
	}
	if val.Kind() != contracts.Text {
		return contracts.Value{}, contracts.ErrUnsupported
	}
	runes := []rune(source)
	out := make([]rune, len(runes))
	changed := false
	for i, original := range runes {
		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], uint64(i))
		message := append(append(make([]byte, 0, 40), seed[:]...), counter[:]...)
		block := hmacSHA256([]byte(key.Payload()), message)
		choices := ""
		switch {
		case original >= 'a' && original <= 'z':
			choices = "abcdefghijklmnopqrstuvwxyz"
		case original >= 'A' && original <= 'Z':
			choices = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		case original >= '0' && original <= '9':
			choices = "0123456789"
		case unicode.Is(unicode.Han, original):
			choices = "天地玄黄宇宙洪荒日月盈昃辰宿列张山川草木风云光明华夏星海新梦"
		case unicode.IsSpace(original), unicode.IsPunct(original), unicode.IsSymbol(original):
			out[i] = original // Preserve separators, space and emoji.
			continue
		default:
			return contracts.Value{}, contracts.ErrUnsupported
		}
		alphabet := []rune(choices)
		index := -1
		for j, symbol := range alphabet {
			if symbol == original {
				index = j
				break
			}
		}
		if index < 0 {
			out[i] = alphabet[int(block[0])%len(alphabet)]
		} else {
			// Always select a different member of the same character class.
			out[i] = alphabet[(index+1+int(block[0])%(len(alphabet)-1))%len(alphabet)]
		}
		changed = changed || out[i] != original
	}
	if !changed {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return contracts.NewValue(contracts.Text, string(out))
}
