package history

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestLookupRejectsFutureIssuedRecord pins the future-issued bound: a validly
// signed record whose IssuedAt is later than the reader's clock plus
// maxFutureIssuedSkew must be refused, while a small clock difference is
// tolerated. Deleting the IssuedAt comparison in checkActive must fail this
// test.
func TestLookupRejectsFutureIssuedRecord(t *testing.T) {
	t.Run("issued an hour ahead", func(t *testing.T) {
		f := newFixture(t)
		future := f.now.Add(time.Hour)
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
			r.IssuedAt = future
			r.ExpiresAt = future.Add(time.Hour)
		}), f.private)
		_, err := f.lookup(f.plan)
		requireCategory(t, err, contracts.ErrUnsafe)
		requireCause(t, err, errFutureIssued)
	})
	t.Run("issued one second beyond the tolerance", func(t *testing.T) {
		f := newFixture(t)
		future := f.now.Add(maxFutureIssuedSkew + time.Second)
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
			r.IssuedAt = future
			r.ExpiresAt = future.Add(time.Hour)
		}), f.private)
		_, err := f.lookup(f.plan)
		requireCategory(t, err, contracts.ErrUnsafe)
		requireCause(t, err, errFutureIssued)
	})
	t.Run("issued inside the tolerance", func(t *testing.T) {
		f := newFixture(t)
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
			r.IssuedAt = f.now.Add(maxFutureIssuedSkew)
			r.ExpiresAt = f.now.Add(time.Hour)
		}), f.private)
		if _, err := f.lookup(f.plan); err != nil {
			t.Fatalf("Lookup() at the tolerance boundary error = %v", err)
		}
	})
	t.Run("reader clock slightly behind the issuer", func(t *testing.T) {
		f := newFixture(t)
		issued := f.now
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
			r.IssuedAt = issued
			r.ExpiresAt = issued.Add(time.Hour)
		}), f.private)
		f.now = issued.Add(-time.Minute)
		if _, err := f.lookup(f.plan); err != nil {
			t.Fatalf("Lookup() with a one-minute clock difference error = %v", err)
		}
	})
}

// TestOpenRejectsPrivateKeyThatDoesNotMatchTrustedKey is the PART 0 regression
// for the signer/trust correspondence. A store whose SignerKeyID is trusted but
// whose private key is a different key would sign records no verifier accepts,
// so both Open and Approve must refuse it. Removing either check must fail this
// test.
func TestOpenRejectsPrivateKeyThatDoesNotMatchTrustedKey(t *testing.T) {
	f := newFixture(t)
	wrong, wrongPublic := testKey(t, 0x77)
	if public, _ := f.private.Public().(ed25519.PublicKey); string(public) == string(wrongPublic) {
		t.Fatal("fixture keys must differ")
	}
	_, err := Open(Config{Dir: f.dir, Trust: f.trust,
		Signer: &Signer{SignerKeyID: reviewerKeyID, Private: wrong},
		Now:    func() time.Time { return f.now }})
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errSignerMismatch)
	if _, statErr := os.Stat(f.dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("refused Open created %s", f.dir)
	}

	// A private key whose two halves disagree would open successfully and then
	// sign a record no verifier accepts, so the correspondence check covers it.
	inconsistent := append(ed25519.PrivateKey(nil), f.private...)
	inconsistent[ed25519.SeedSize] ^= 0x01
	if _, err := Open(Config{Dir: f.dir, Trust: f.trust,
		Signer: &Signer{SignerKeyID: reviewerKeyID, Private: inconsistent},
		Now:    func() time.Time { return f.now }}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Open(inconsistent key) error = %v, want ErrUnsafe", err)
	}

	// The Approve-side check is a second, independent gate: a Store assembled
	// without Open must not persist a record for a key the trust set does not
	// own either.
	direct := &Store{dir: f.dir, trust: f.trust, now: func() time.Time { return f.now }, rename: os.Rename,
		signer: &Signer{SignerKeyID: reviewerKeyID, Private: wrong}}
	_, err = direct.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errSignerMismatch)
	if _, statErr := os.Stat(f.dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("refused Approve created %s", f.dir)
	}
}

func TestLoadTrustFile(t *testing.T) {
	_, public := testKey(t, 0x21)
	valid := `{"version":"` + TrustFileVersion + `","keys":[{"signer_key_id":"` +
		reviewerKeyID + `","public_key_hex":"` + hex.EncodeToString(public) + `"}]}`
	writeFile := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "trust.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("valid", func(t *testing.T) {
		trust, err := LoadTrustFile(writeFile(t, valid))
		if err != nil {
			t.Fatalf("LoadTrustFile() error = %v", err)
		}
		if _, ok := trust.lookup(reviewerKeyID); !ok {
			t.Fatal("loaded trust set does not hold the configured key")
		}
	})
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"unknown field", `{"version":"` + TrustFileVersion + `","extra":1,"keys":[]}`},
		{"wrong version", `{"version":"maskriver-trust-v2","keys":[{"signer_key_id":"k","public_key_hex":"` + hex.EncodeToString(public) + `"}]}`},
		{"no keys", `{"version":"` + TrustFileVersion + `","keys":[]}`},
		{"empty key id", `{"version":"` + TrustFileVersion + `","keys":[{"signer_key_id":"","public_key_hex":"` + hex.EncodeToString(public) + `"}]}`},
		{"bad hex", `{"version":"` + TrustFileVersion + `","keys":[{"signer_key_id":"k","public_key_hex":"zz"}]}`},
		{"short key", `{"version":"` + TrustFileVersion + `","keys":[{"signer_key_id":"k","public_key_hex":"0011"}]}`},
		{"duplicate id", `{"version":"` + TrustFileVersion + `","keys":[` +
			`{"signer_key_id":"k","public_key_hex":"` + hex.EncodeToString(public) + `"},` +
			`{"signer_key_id":"k","public_key_hex":"` + hex.EncodeToString(public) + `"}]}`},
		{"trailing value", valid + `{}`},
		{"not json", "maskriver trust"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadTrustFile(writeFile(t, test.content)); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("LoadTrustFile() error = %v, want ErrInvalid", err)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadTrustFile(filepath.Join(t.TempDir(), "absent.json")); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatalf("LoadTrustFile(missing) error = %v, want ErrInvalid", err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		if _, err := LoadTrustFile(t.TempDir()); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatalf("LoadTrustFile(directory) error = %v, want ErrInvalid", err)
		}
	})
}

func TestLoadSignerKeyFile(t *testing.T) {
	private, public := testKey(t, 0x31)
	writeFile := func(t *testing.T, raw []byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "reviewer.key")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	derive := func(t *testing.T, key ed25519.PrivateKey) ed25519.PublicKey {
		t.Helper()
		derived, ok := key.Public().(ed25519.PublicKey)
		if !ok {
			t.Fatal("private key did not expose a public key")
		}
		return derived
	}
	t.Run("seed", func(t *testing.T) {
		key, err := LoadSignerKeyFile(writeFile(t, private.Seed()))
		if err != nil {
			t.Fatalf("LoadSignerKeyFile(seed) error = %v", err)
		}
		if string(derive(t, key)) != string(public) {
			t.Fatal("loaded seed did not derive the configured public key")
		}
	})
	t.Run("private key", func(t *testing.T) {
		key, err := LoadSignerKeyFile(writeFile(t, private))
		if err != nil {
			t.Fatalf("LoadSignerKeyFile(private) error = %v", err)
		}
		if string(derive(t, key)) != string(public) {
			t.Fatal("loaded key did not derive the configured public key")
		}
	})
	t.Run("inconsistent private key", func(t *testing.T) {
		corrupted := append([]byte(nil), private...)
		corrupted[ed25519.SeedSize] ^= 0x01
		if _, err := LoadSignerKeyFile(writeFile(t, corrupted)); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatalf("LoadSignerKeyFile(inconsistent) error = %v, want ErrInvalid", err)
		}
	})
	for _, size := range []int{0, 1, 31, 33, 63, 65} {
		if _, err := LoadSignerKeyFile(writeFile(t, make([]byte, size))); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatalf("LoadSignerKeyFile(%d bytes) error = %v, want ErrInvalid", size, err)
		}
	}
	if _, err := LoadSignerKeyFile(filepath.Join(t.TempDir(), "absent.key")); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("LoadSignerKeyFile(missing) error = %v, want ErrInvalid", err)
	}
}
