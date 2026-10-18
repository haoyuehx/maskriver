package history

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	// recordSuffix makes one plan map to exactly one file name.
	recordSuffix = ".decision"
	// maxRecordBytes bounds what the store will read or write. A record is a few
	// kilobytes; the bound stops a hostile file from exhausting memory.
	maxRecordBytes = 1 << 20
	// maxPlanIDBytes bounds the plan identity the store will turn into a path.
	maxPlanIDBytes = 128
	// storeDirPerm and storeFilePerm keep records readable only by their owner.
	storeDirPerm  fs.FileMode = 0o700
	storeFilePerm fs.FileMode = 0o600
)

// Signer is the review signing key, supplied by the caller from outside the
// record. SignerKeyID must also be a trusted key: the store refuses to sign a
// record it could not itself verify.
type Signer struct {
	SignerKeyID string
	Private     ed25519.PrivateKey
}

// TrustedKey is one public key the store will accept signatures from, with the
// identity the record must name.
type TrustedKey struct {
	SignerKeyID string
	Public      ed25519.PublicKey
}

// TrustedKeys is the explicit trust set for a store. An empty set is refused at
// Open: a store that trusts nobody must fail closed rather than accept records.
type TrustedKeys struct {
	keys map[string]ed25519.PublicKey
}

// NewTrustedKeys validates and copies an explicit trust set. Duplicate signer
// identities are refused because they would make "which key signed this"
// ambiguous.
func NewTrustedKeys(keys ...TrustedKey) (TrustedKeys, error) {
	trusted := TrustedKeys{keys: make(map[string]ed25519.PublicKey, len(keys))}
	for _, key := range keys {
		if key.SignerKeyID == "" || len(key.SignerKeyID) > maxIdentityBytes ||
			len(key.Public) != ed25519.PublicKeySize {
			return TrustedKeys{}, contracts.ErrInvalid
		}
		if _, exists := trusted.keys[key.SignerKeyID]; exists {
			return TrustedKeys{}, contracts.ErrInvalid
		}
		trusted.keys[key.SignerKeyID] = append(ed25519.PublicKey(nil), key.Public...)
	}
	return trusted, nil
}

func (t TrustedKeys) lookup(signerKeyID string) (ed25519.PublicKey, bool) {
	public, ok := t.keys[signerKeyID]
	return public, ok
}

// Config configures a Store. Trust and Signer are supplied by the caller; no key
// material is ever read from or written to the store.
type Config struct {
	Dir string
	// Trust is the set of reviewer public keys. It must not be empty.
	Trust TrustedKeys
	// Signer is nil for a read-only store; Approve and Revoke then refuse.
	Signer *Signer
	// Now defaults to time.Now and is the clock used for expiry checks.
	Now func() time.Time
}

// DecisionStore is the explicit review-decision surface for Main to inject. It
// has exactly one write path per operation: Approve and Revoke persist, Lookup
// never writes. Scan, preview, validate and plan building must not reach the
// write side, so nothing here writes during Open or Lookup.
type DecisionStore interface {
	Approve(context.Context, ApproveRequest) (Record, error)
	Revoke(context.Context, RevokeRequest) (Record, error)
	Lookup(context.Context, contracts.Plan) (Record, error)
}

var _ DecisionStore = (*Store)(nil)

// Store is a local file store of signed reviewer decisions. It performs no I/O
// until an operation is called and it holds no open handles.
type Store struct {
	dir    string
	trust  TrustedKeys
	signer *Signer
	now    func() time.Time
	// rename is a test seam for an interrupted commit; it is os.Rename in
	// production and is never nil.
	rename func(string, string) error
}

// ApproveRequest asks to persist one decision. Every bound field comes from Plan,
// so the record always covers the whole plan: there is no partial approval. The
// caller states issued-at and expiry explicitly; a window that is empty or
// already over is refused.
type ApproveRequest struct {
	Plan       contracts.Plan
	ReviewerID string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// RevokeRequest asks to mark an existing record revoked. RevokedAt is required so
// the caller states when; the store does not invent the time.
type RevokeRequest struct {
	Plan      contracts.Plan
	RevokedAt time.Time
}

// Open builds a store. It validates configuration only: it does not create,
// read or list the directory, so opening a store in a read-only flow leaves no
// trace on disk.
func Open(cfg Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, contracts.ErrInvalid
	}
	if len(cfg.Trust.keys) == 0 {
		return nil, unsafeError(errNoTrustedKey)
	}
	store := &Store{
		dir: cfg.Dir, trust: cfg.Trust, now: cfg.Now, rename: os.Rename,
	}
	if store.now == nil {
		store.now = time.Now
	}
	if cfg.Signer != nil {
		if cfg.Signer.SignerKeyID == "" ||
			len(cfg.Signer.SignerKeyID) > maxIdentityBytes ||
			len(cfg.Signer.Private) != ed25519.PrivateKeySize {
			return nil, contracts.ErrInvalid
		}
		if _, trusted := cfg.Trust.lookup(cfg.Signer.SignerKeyID); !trusted {
			return nil, unsafeError(errUnknownSigner)
		}
		// A matching signer key ID is a label, not proof: the supplied private
		// key must actually be the key the trust set registers under that ID.
		// Without this check the store would open and then persist a record no
		// verifier can accept, which is a silent, unusable approval.
		if !signerMatchesTrust(cfg.Signer, cfg.Trust) {
			return nil, unsafeError(errSignerMismatch)
		}
		signer := Signer{
			SignerKeyID: cfg.Signer.SignerKeyID,
			Private:     append(ed25519.PrivateKey(nil), cfg.Signer.Private...),
		}
		store.signer = &signer
	}
	return store, nil
}

// Approve signs and persists a decision for the whole plan. Writing is explicit:
// this is the only path that creates a record. Re-approving the same plan
// replaces the previous record (last writer wins, see package doc).
func (s *Store) Approve(ctx context.Context, req ApproveRequest) (Record, error) {
	if ctx == nil {
		return Record{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if s.signer == nil {
		return Record{}, unsafeError(errNoSigner)
	}
	// Re-check the same correspondence Open enforced, so no alternative
	// construction of a Store can sign with a key the trust set does not own.
	if !signerMatchesTrust(s.signer, s.trust) {
		return Record{}, unsafeError(errSignerMismatch)
	}
	record, err := newRecord(req.Plan, req.ReviewerID, req.IssuedAt, req.ExpiresAt, s.signer.SignerKeyID)
	if err != nil {
		return Record{}, err
	}
	if !s.now().Before(record.ExpiresAt) {
		return Record{}, unsafeError(errExpired)
	}
	if err := s.requireUnambiguous(req.Plan.ID); err != nil {
		return Record{}, err
	}
	if err := s.write(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Revoke re-signs an existing record with a revocation timestamp. It requires the
// store's signing key to be the key that signed the record, and it cannot revoke
// a record whose signature or binding already fails. Revocation state lives in
// the same file, so rolling that file back rolls the revocation back; see the
// package doc.
func (s *Store) Revoke(ctx context.Context, req RevokeRequest) (Record, error) {
	if ctx == nil {
		return Record{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if s.signer == nil {
		return Record{}, unsafeError(errNoSigner)
	}
	if req.RevokedAt.IsZero() {
		return Record{}, contracts.ErrInvalid
	}
	record, err := s.load(ctx, req.Plan)
	if err != nil {
		return Record{}, err
	}
	if record.SignerKeyID != s.signer.SignerKeyID {
		return Record{}, unsafeError(errForeignSigner)
	}
	if record.RevokedAt != nil {
		return Record{}, conflictError(errAlreadyRevoked)
	}
	revokedAt := req.RevokedAt.UTC()
	record.RevokedAt = &revokedAt
	if err := s.write(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// signerMatchesTrust reports whether the private key behind a signer derives the
// public key the trust set registers under the signer's key ID, and whether the
// key's own public half is that same key. Checking both matters: Ed25519 signs
// from the seed but embeds the public half, so a key whose halves disagree would
// open successfully and then produce a signature no verifier accepts.
func signerMatchesTrust(signer *Signer, trust TrustedKeys) bool {
	if signer == nil || len(signer.Private) != ed25519.PrivateKeySize {
		return false
	}
	public, trusted := trust.lookup(signer.SignerKeyID)
	if !trusted {
		return false
	}
	derived, ok := ed25519.NewKeyFromSeed(signer.Private.Seed()).Public().(ed25519.PublicKey)
	if !ok || len(derived) != len(public) || !bytes.Equal(derived, public) {
		return false
	}
	return bytes.Equal(signer.Private[ed25519.SeedSize:], public)
}

// Lookup returns the record for this plan only if it is authentic, bound to
// exactly this plan, not issued beyond the tolerated clock skew, unexpired and
// not revoked. It never writes.
func (s *Store) Lookup(ctx context.Context, plan contracts.Plan) (Record, error) {
	if ctx == nil {
		return Record{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	record, err := s.load(ctx, plan)
	if err != nil {
		return Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := record.checkActive(s.now()); err != nil {
		return Record{}, err
	}
	return record, nil
}

// load verifies authenticity and binding, but not expiry or revocation: revoking
// an expired record is legitimate, so Revoke uses this narrower gate.
func (s *Store) load(ctx context.Context, plan contracts.Plan) (Record, error) {
	if err := plan.ValidateShape(); err != nil {
		return Record{}, err
	}
	path, err := recordPath(s.dir, plan.ID)
	if err != nil {
		return Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := s.requireUnambiguous(plan.ID); err != nil {
		return Record{}, err
	}
	raw, err := readCapped(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Record{}, contracts.ErrNotFound
		}
		if errors.Is(err, contracts.ErrUnsafe) {
			return Record{}, err
		}
		return Record{}, unsafeError(errRead)
	}
	record, err := s.verify(raw)
	if err != nil {
		return Record{}, err
	}
	if err := record.checkBinding(plan); err != nil {
		return Record{}, err
	}
	return record, nil
}

// verify checks the detached signature with the trusted key the payload names.
func (s *Store) verify(raw []byte) (Record, error) {
	record, payload, signature, err := decodeRecord(raw)
	if err != nil {
		return Record{}, err
	}
	public, trusted := s.trust.lookup(record.SignerKeyID)
	if !trusted {
		return Record{}, unsafeError(errUnknownSigner)
	}
	if !ed25519.Verify(public, payload, signature) {
		return Record{}, unsafeError(errSignature)
	}
	return record, nil
}

// requireUnambiguous refuses to resolve a plan when the directory holds more than
// one decodable record claiming it. A second copy would otherwise make "which
// record applies" depend on the file name an attacker chose. Files that do not
// declare a plan id can never be loaded as a record and are ignored.
func (s *Store) requireUnambiguous(planID string) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return unsafeError(errRead)
	}
	canonical := planID + recordSuffix
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == canonical ||
			!strings.HasSuffix(entry.Name(), recordSuffix) {
			continue
		}
		raw, err := readCapped(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		if claimed, ok := claimedPlanID(raw); ok && claimed == planID {
			return conflictError(errAmbiguous)
		}
	}
	return nil
}

// write persists one record: stage a complete file in the same directory, fsync
// it, rename it over the target and fsync the directory. A crash between the
// steps leaves either the previous record or the new one, never a partial file,
// and the caller is told the write failed.
func (s *Store) write(ctx context.Context, record Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := encodeRecord(record, s.signer.Private)
	if err != nil {
		return err
	}
	target, err := recordPath(s.dir, record.PlanID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, storeDirPerm); err != nil {
		return unsafeError(errWrite)
	}
	staged, err := os.CreateTemp(s.dir, "."+filepath.Base(target)+".tmp-")
	if err != nil {
		return unsafeError(errWrite)
	}
	stagedName := staged.Name()
	discard := func() {
		_ = staged.Close()
		_ = os.Remove(stagedName)
	}
	// CreateTemp already uses 0600; set it explicitly so the invariant does not
	// depend on a standard-library default.
	if err := staged.Chmod(storeFilePerm); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if _, err := staged.Write(raw); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if err := staged.Sync(); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedName)
		return unsafeError(errWrite)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(stagedName)
		return err
	}
	if err := s.renameIntoPlace(stagedName, target, raw); err != nil {
		_ = os.Remove(stagedName)
		return unsafeError(errWrite)
	}
	return s.syncDir()
}

// renameIntoPlace makes the staged file the record. The store has no lock file
// and is last-writer-wins, so a concurrent write may replace the target first.
// On Windows a replace can also fail transiently with a sharing violation while
// another handle is open, which is not a durable failure: retry briefly, and if
// the target already holds exactly these bytes the write already succeeded.
func (s *Store) renameIntoPlace(stagedName, target string, raw []byte) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := s.rename(stagedName, target); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if existing, err := os.ReadFile(target); err == nil && bytesEqual(existing, raw) {
			_ = os.Remove(stagedName)
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return lastErr
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// syncDir durably records the rename on platforms that can flush a directory.
// Windows rejects Sync on a directory handle, and some filesystems do not
// support it at all; there the file itself was already flushed and the rename
// is left to the platform. This is a durability reduction that must be visible
// rather than a hard failure that would make the store unusable on Windows.
func (s *Store) syncDir() error {
	dir, err := os.Open(s.dir)
	if err != nil {
		return unsafeError(errWrite)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) ||
			errors.Is(err, syscall.EBADF) || runtime.GOOS == "windows" {
			return nil
		}
		return unsafeError(errWrite)
	}
	return nil
}

// recordPath maps a plan id to its single file, refusing any id that is not one
// safe path element so a hostile identifier cannot address another file.
func recordPath(dir, planID string) (string, error) {
	if !safePlanID(planID) {
		return "", contracts.ErrInvalid
	}
	return filepath.Join(dir, planID+recordSuffix), nil
}

func safePlanID(planID string) bool {
	if planID == "" || len(planID) > maxPlanIDBytes {
		return false
	}
	for _, char := range planID {
		switch {
		case char >= '0' && char <= '9', char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z', char == '-', char == '_', char == '.', char == ':':
		default:
			return false
		}
	}
	return true
}

// readCapped reads at most maxRecordBytes and reports the store's own bounds;
// os/fs errors are returned unwrapped so callers can classify not-exist.
func readCapped(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRecordBytes {
		return nil, unsafeError(errCorrupt)
	}
	return raw, nil
}
