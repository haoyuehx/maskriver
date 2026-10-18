package history

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestLookupRejectsForgedSignature signs a record with a key the record does not
// name: the signature is well formed, the named key is trusted, and it still must
// not verify.
func TestLookupRejectsForgedSignature(t *testing.T) {
	f := newFixture(t)
	forged, _ := testKey(t, 0x33)
	f.writeRecord(t, f.plan.ID, f.buildRecord(t, nil), forged)
	_, err := f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errSignature)
}

// TestLookupRejectsUnknownSigner keeps the signature valid for the key that made
// it, but that key is not in the trust set.
func TestLookupRejectsUnknownSigner(t *testing.T) {
	f := newFixture(t)
	unknown, _ := testKey(t, 0x44)
	f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
		r.SignerKeyID = "reviewer-key-untrusted"
	}), unknown)
	_, err := f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errUnknownSigner)
}

// TestLookupRejectsTamperedByte flips one bit at every byte offset of a valid
// stored file. Every single-byte change must be refused, which is the guarantee
// the detached signature plus the canonical form are there to provide.
func TestLookupRejectsTamperedByte(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	path := filepath.Join(f.dir, f.plan.ID+recordSuffix)
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	for offset := range valid {
		tampered := append([]byte(nil), valid...)
		tampered[offset] ^= 0x01
		if bytes.Equal(tampered, valid) {
			t.Fatalf("offset %d: tampered copy equals the original", offset)
		}
		if err := os.WriteFile(path, tampered, storeFilePerm); err != nil {
			t.Fatalf("write tampered record = %v", err)
		}
		if _, err := f.lookup(f.plan); !errors.Is(err, contracts.ErrUnsafe) && !errors.Is(err, contracts.ErrConflict) {
			t.Fatalf("offset %d: Lookup() error = %v, want a refusal", offset, err)
		}
	}
	if err := os.WriteFile(path, valid, storeFilePerm); err != nil {
		t.Fatalf("restore record = %v", err)
	}
	if _, err := f.lookup(f.plan); err != nil {
		t.Fatalf("restored record rejected: %v", err)
	}
}

func TestLookupRejectsExpiredRecord(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	if _, err := f.lookup(f.plan); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	f.now = f.now.Add(24 * time.Hour) // expiry instant is inclusive
	_, err := f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errExpired)
}

func TestRevokeMakesRecordUnusable(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	if _, err := f.store.Revoke(context.Background(), RevokeRequest{Plan: f.plan, RevokedAt: f.now}); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	_, err := f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errRevoked)

	// A second revocation is a conflict, not a silent rewrite.
	_, err = f.store.Revoke(context.Background(), RevokeRequest{Plan: f.plan, RevokedAt: f.now})
	requireCategory(t, err, contracts.ErrConflict)
	requireCause(t, err, errAlreadyRevoked)
}

func TestRevokeRequiresTheApprovingKey(t *testing.T) {
	f := newFixture(t)
	// The record is signed by another trusted key; this store's signer is not it.
	other, otherPublic := testKey(t, 0x55)
	trust, err := NewTrustedKeys(
		TrustedKey{SignerKeyID: reviewerKeyID, Public: f.public},
		TrustedKey{SignerKeyID: "reviewer-key-2", Public: otherPublic},
	)
	if err != nil {
		t.Fatalf("NewTrustedKeys() error = %v", err)
	}
	store, err := Open(Config{Dir: f.dir, Trust: trust,
		Signer: &Signer{SignerKeyID: reviewerKeyID, Private: f.private},
		Now:    func() time.Time { return f.now }})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	record, err := newRecord(f.plan, reviewerLabel, f.now, f.now.Add(time.Hour), "reviewer-key-2")
	if err != nil {
		t.Fatalf("newRecord() error = %v", err)
	}
	f.writeRecord(t, f.plan.ID, record, other)
	_, err = store.Revoke(context.Background(), RevokeRequest{Plan: f.plan, RevokedAt: f.now})
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errForeignSigner)
}

func TestRevokeRejectsBadRequests(t *testing.T) {
	f := newFixture(t)
	if _, err := f.store.Revoke(context.Background(), RevokeRequest{Plan: f.plan}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Revoke(zero time) error = %v, want ErrInvalid", err)
	}
	if _, err := f.store.Revoke(context.Background(), RevokeRequest{Plan: f.plan, RevokedAt: f.now}); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("Revoke(no record) error = %v, want ErrNotFound", err)
	}
}

// TestLookupRejectsAmbiguousRecords refuses to pick a winner when two decodable
// records claim the same plan, and it refuses the same way every time.
func TestLookupRejectsAmbiguousRecords(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	valid, err := os.ReadFile(filepath.Join(f.dir, f.plan.ID+recordSuffix))
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	duplicate := filepath.Join(f.dir, "duplicate"+recordSuffix)
	if err := os.WriteFile(duplicate, valid, storeFilePerm); err != nil {
		t.Fatalf("write duplicate = %v", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		_, err := f.lookup(f.plan)
		requireCategory(t, err, contracts.ErrConflict)
		requireCause(t, err, errAmbiguous)
	}
	if _, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("Approve(ambiguous store) error = %v, want ErrConflict", err)
	}
	if err := os.Remove(duplicate); err != nil {
		t.Fatalf("remove duplicate = %v", err)
	}
	if _, err := f.lookup(f.plan); err != nil {
		t.Fatalf("Lookup() after removing duplicate error = %v", err)
	}
}

// TestLookupIgnoresFilesThatCannotBeRecords keeps unrelated files in the store
// directory from breaking reads, while still refusing a real second record.
func TestLookupIgnoresFilesThatCannotBeRecords(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	if err := os.WriteFile(filepath.Join(f.dir, "notes.txt"), []byte("synthetic note"), storeFilePerm); err != nil {
		t.Fatalf("write note = %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "broken"+recordSuffix), []byte("not a record"), storeFilePerm); err != nil {
		t.Fatalf("write broken file = %v", err)
	}
	if _, err := f.lookup(f.plan); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
}

func TestEncodeRejectsShortKey(t *testing.T) {
	if _, err := encodeRecord(Record{}, ed25519.PrivateKey("short")); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("encodeRecord(short key) error = %v, want ErrUnsafe", err)
	}
}

func TestOversizedRecordRejected(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	path := filepath.Join(f.dir, f.plan.ID+recordSuffix)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	oversized := append(raw, bytes.Repeat([]byte{' '}, maxRecordBytes)...)
	if err := os.WriteFile(path, oversized, storeFilePerm); err != nil {
		t.Fatalf("write oversized record = %v", err)
	}
	_, err = f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrUnsafe)
}
