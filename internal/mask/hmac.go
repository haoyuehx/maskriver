package mask

import (
	"crypto/hmac"
	"crypto/sha256"
)

// hmacSHA256 is a tiny thin wrapper so that call sites in strategy.go
// do not directly import crypto packages; it also ensures the caller
// always copies the returned 32-byte digest.
func hmacSHA256(key, msg []byte) [32]byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}
