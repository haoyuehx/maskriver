package history

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file loads the two pieces of deployment configuration the review CLI
// needs: the trust set of reviewer public keys and one reviewer private key.
// Both are files on disk rather than command-line values, so an apply-time
// invocation points at a pre-provisioned trust set instead of naming the key it
// wants to trust. It is still a local file boundary: an operator who can write
// both the trust file and the reviewer key file has not been stopped by the
// tool, and this package does not claim otherwise (see the package doc).

const (
	// TrustFileVersion is the trust-file format version. It is independent of
	// RecordVersion and of the frozen contracts revision.
	TrustFileVersion = "maskriver-trust-v1"
	// maxTrustFileBytes bounds what a trust file may make the loader read.
	maxTrustFileBytes = 1 << 20
	// maxPrivateKeyFileBytes bounds the reviewer key file. A key is 32 or 64
	// bytes; the bound only has to reject an obviously wrong file.
	maxPrivateKeyFileBytes = 4096
	// maxTrustedKeyIDBytes is the same identity bound records enforce.
	maxTrustedKeyIDBytes = maxIdentityBytes
)

// trustFile is the on-disk trust configuration. Field names are its format.
type trustFile struct {
	Version string         `json:"version"`
	Keys    []trustFileKey `json:"keys"`
}

type trustFileKey struct {
	SignerKeyID  string `json:"signer_key_id"`
	PublicKeyHex string `json:"public_key_hex"`
}

// LoadTrustFile reads a trust set from a deployment-provided file. Unknown
// fields, a wrong version, an empty key list, a malformed or non-Ed25519 public
// key and a duplicate signer identity are all refused; every error is sanitized
// and never names the file path or its contents. An empty trust set is refused,
// because a store that trusts nobody must fail closed.
func LoadTrustFile(path string) (TrustedKeys, error) {
	raw, err := readRegularFile(path, maxTrustFileBytes)
	if err != nil {
		return TrustedKeys{}, err
	}
	var wire trustFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return TrustedKeys{}, contracts.ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return TrustedKeys{}, contracts.ErrInvalid
	}
	if wire.Version != TrustFileVersion || len(wire.Keys) == 0 {
		return TrustedKeys{}, contracts.ErrInvalid
	}
	keys := make([]TrustedKey, 0, len(wire.Keys))
	for _, entry := range wire.Keys {
		if entry.SignerKeyID == "" || len(entry.SignerKeyID) > maxTrustedKeyIDBytes {
			return TrustedKeys{}, contracts.ErrInvalid
		}
		public, err := hex.DecodeString(entry.PublicKeyHex)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return TrustedKeys{}, contracts.ErrInvalid
		}
		keys = append(keys, TrustedKey{
			SignerKeyID: entry.SignerKeyID, Public: ed25519.PublicKey(public),
		})
	}
	return NewTrustedKeys(keys...)
}

// LoadSignerKeyFile reads one reviewer Ed25519 private key. A 32-byte file is
// treated as an Ed25519 seed and a 64-byte file as a complete private key; any
// other size is refused. The key material is never included in an error and the
// caller must not copy it into a record or a log.
func LoadSignerKeyFile(path string) (ed25519.PrivateKey, error) {
	raw, err := readRegularFile(path, maxPrivateKeyFileBytes)
	if err != nil {
		return nil, err
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		key := ed25519.PrivateKey(append([]byte(nil), raw...))
		// A 64-byte key carries a seed and its public half. Refuse a file whose
		// halves disagree: signing with it yields a signature that never verifies.
		if !bytes.Equal(ed25519.NewKeyFromSeed(key.Seed()), key) {
			return nil, contracts.ErrInvalid
		}
		return key, nil
	default:
		return nil, contracts.ErrInvalid
	}
}

// readRegularFile reads one bounded, regular file. Missing, unreadable,
// oversized and non-regular files all collapse to contracts.ErrInvalid so no
// caller can leak a path or distinguish filesystem state through the error.
func readRegularFile(path string, limit int64) ([]byte, error) {
	if path == "" {
		return nil, contracts.ErrInvalid
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, contracts.ErrInvalid
	}
	raw, err := os.ReadFile(path)
	if err != nil || int64(len(raw)) > limit {
		return nil, contracts.ErrInvalid
	}
	return raw, nil
}
