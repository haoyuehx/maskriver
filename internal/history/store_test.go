package history

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	reviewerKeyID = "reviewer-key-1"
	reviewerLabel = "reviewer@synthetic.example"
)

// testClock is the fixed store clock; expiry tests move fixture.now instead of
// waiting, so no test depends on wall time.
var testClock = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fixture is a store over a synthetic two-table plan plus the key material the
// tests inject. All data here is synthetic and generated per test.
type fixture struct {
	store    *Store
	dir      string
	plan     contracts.Plan
	trust    TrustedKeys
	keyID    string
	private  ed25519.PrivateKey
	public   ed25519.PublicKey
	now      time.Time
	testRoot string
}

func testKey(t *testing.T, seed byte) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	seedBytes := bytes.Repeat([]byte{seed}, ed25519.SeedSize)
	private := ed25519.NewKeyFromSeed(seedBytes)
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatal("ed25519 private key did not expose a public key")
	}
	return private, public
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	private, public := testKey(t, 0x11)
	trust, err := NewTrustedKeys(TrustedKey{SignerKeyID: reviewerKeyID, Public: public})
	if err != nil {
		t.Fatalf("NewTrustedKeys() error = %v", err)
	}
	root := t.TempDir()
	f := &fixture{
		dir: filepath.Join(root, "history"), plan: syntheticPlan(t), trust: trust,
		keyID: reviewerKeyID, private: private, public: public, now: testClock,
		testRoot: root,
	}
	f.openStore(t)
	return f
}

// openStore builds the store with the fixture clock so expiry can be simulated.
func (f *fixture) openStore(t *testing.T) {
	t.Helper()
	store, err := Open(Config{
		Dir:    f.dir,
		Trust:  f.trust,
		Signer: &Signer{SignerKeyID: f.keyID, Private: f.private},
		Now:    func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	f.store = store
}

func syntheticPlan(t *testing.T) contracts.Plan {
	t.Helper()
	people := contracts.TableRef{Database: "synthetic", Schema: "main", Table: "people"}
	orders := contracts.TableRef{Database: "synthetic", Schema: "main", Table: "orders"}
	column := func(table contracts.TableRef, name string) contracts.ColumnPlan {
		return contracts.ColumnPlan{
			Column:               contracts.ColumnRef{TableRef: table, Column: name},
			Type:                 contracts.Type{Kind: contracts.Text, Native: "TEXT", Nullable: true},
			Strategy:             contracts.StrategyRef{ID: "email", Version: "1"},
			Scope:                "sqlite/" + table.Table + "/" + name,
			NormalizationVersion: "typed-lexical-v1",
			KeyID:                "key-1",
		}
	}
	plan := contracts.Plan{
		Revision:         contracts.APIRevision,
		ID:               "plan-digest-1",
		SourceSnapshotID: "source-digest-1",
		TargetID:         "target-digest-1",
		SchemaDigest:     "schema-digest-1",
		ConfigDigest:     "config-digest-1",
		ScanComplete:     true,
		Tables: []contracts.TablePlan{
			{Table: people, PrimaryKey: []string{"id"}, Columns: []contracts.ColumnPlan{
				column(people, "email"),
			}},
			{Table: orders, PrimaryKey: []string{"id"}, Columns: []contracts.ColumnPlan{
				column(orders, "email"), column(orders, "contact"),
			}},
		},
	}
	if err := plan.ValidateShape(); err != nil {
		t.Fatalf("synthetic plan is not valid: %v", err)
	}
	return plan
}

func clonePlan(plan contracts.Plan) contracts.Plan {
	clone := plan
	clone.Tables = make([]contracts.TablePlan, len(plan.Tables))
	for index, table := range plan.Tables {
		clone.Tables[index] = table
		clone.Tables[index].PrimaryKey = append([]string(nil), table.PrimaryKey...)
		clone.Tables[index].Columns = append([]contracts.ColumnPlan(nil), table.Columns...)
	}
	return clone
}

func (f *fixture) approve(t *testing.T) Record {
	t.Helper()
	record, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel,
		IssuedAt: f.now, ExpiresAt: f.now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	return record
}

// buildRecord derives a record for the fixture plan without storing it, so tests
// can mutate a single bound field and still present a valid signature.
func (f *fixture) buildRecord(t *testing.T, mutate func(*Record)) Record {
	t.Helper()
	record, err := newRecord(f.plan, reviewerLabel, f.now, f.now.Add(24*time.Hour), f.keyID)
	if err != nil {
		t.Fatalf("newRecord() error = %v", err)
	}
	if mutate != nil {
		mutate(&record)
	}
	return record
}

// writeRecord stores a record under the given plan file name, bypassing Approve,
// which is how a hostile or buggy record reaches the store.
func (f *fixture) writeRecord(t *testing.T, namePlanID string, record Record, private ed25519.PrivateKey) {
	t.Helper()
	raw, err := encodeRecord(record, private)
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}
	if err := os.MkdirAll(f.dir, storeDirPerm); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path, err := recordPath(f.dir, namePlanID)
	if err != nil {
		t.Fatalf("recordPath() error = %v", err)
	}
	if err := os.WriteFile(path, raw, storeFilePerm); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func (f *fixture) lookup(plan contracts.Plan) (Record, error) {
	return f.store.Lookup(context.Background(), plan)
}

func requireCategory(t *testing.T, err error, category error) {
	t.Helper()
	if !errors.Is(err, category) {
		t.Fatalf("error = %v, want category %v", err, category)
	}
}

func requireCause(t *testing.T, err error, cause error) {
	t.Helper()
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want cause %v", err, cause)
	}
}

// treeDigest hashes a file tree's paths, modes and sizes so a read-only flow can
// be shown to leave no file behind.
func treeDigest(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		lines = append(lines, path+"|"+info.Mode().String()+"|"+strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func TestApproveStoresVerifiableRecord(t *testing.T) {
	f := newFixture(t)
	approved := f.approve(t)

	info, err := os.Stat(filepath.Join(f.dir, f.plan.ID+recordSuffix))
	if err != nil {
		t.Fatalf("stat record = %v", err)
	}
	// POSIX permission bits are not modelled on Windows: the file reports 0666
	// and access is governed by inherited ACLs. The 0600 requirement is
	// therefore asserted only where the platform actually stores mode bits.
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != storeFilePerm {
			t.Fatalf("record mode = %o, want %o", mode, storeFilePerm)
		}
	}
	dirInfo, err := os.Stat(f.dir)
	if err != nil {
		t.Fatalf("stat store dir = %v", err)
	}
	if runtime.GOOS != "windows" {
		if mode := dirInfo.Mode().Perm(); mode != storeDirPerm {
			t.Fatalf("store dir mode = %o, want %o", mode, storeDirPerm)
		}
	}

	loaded, err := f.lookup(f.plan)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if loaded.Version != RecordVersion || loaded.PlanID != f.plan.ID ||
		loaded.ReviewerID != reviewerLabel || loaded.SignerKeyID != reviewerKeyID {
		t.Fatalf("loaded record metadata = %+v", loaded)
	}
	if loaded.RevokedAt != nil {
		t.Fatal("fresh record must not be revoked")
	}
	if !loaded.IssuedAt.Equal(approved.IssuedAt) || !loaded.ExpiresAt.Equal(approved.ExpiresAt) {
		t.Fatalf("loaded window = %v..%v, want %v..%v",
			loaded.IssuedAt, loaded.ExpiresAt, approved.IssuedAt, approved.ExpiresAt)
	}
	want := []Scope{
		{Table: contracts.TableRef{Database: "synthetic", Schema: "main", Table: "people"}, Column: "email"},
		{Table: contracts.TableRef{Database: "synthetic", Schema: "main", Table: "orders"}, Column: "email"},
		{Table: contracts.TableRef{Database: "synthetic", Schema: "main", Table: "orders"}, Column: "contact"},
	}
	if len(loaded.Scopes) != len(want) {
		t.Fatalf("scopes = %+v, want %+v", loaded.Scopes, want)
	}
	for index := range want {
		if loaded.Scopes[index] != want[index] {
			t.Fatalf("scope %d = %+v, want %+v", index, loaded.Scopes[index], want[index])
		}
	}
}

func TestUnknownPlanHasNoRecord(t *testing.T) {
	f := newFixture(t)
	_, err := f.lookup(f.plan)
	requireCategory(t, err, contracts.ErrNotFound)
}

func TestOpenRefusesMissingTrustOrInvalidKeys(t *testing.T) {
	if _, err := Open(Config{Dir: t.TempDir()}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Open() without trusted keys error = %v, want ErrUnsafe", err)
	}
	if _, err := Open(Config{Dir: ""}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Open() without dir error = %v, want ErrInvalid", err)
	}
	if _, err := NewTrustedKeys(TrustedKey{SignerKeyID: "k", Public: []byte("short")}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("NewTrustedKeys(short key) error = %v, want ErrInvalid", err)
	}
	_, public := testKey(t, 0x22)
	if _, err := NewTrustedKeys(
		TrustedKey{SignerKeyID: "k", Public: public},
		TrustedKey{SignerKeyID: "k", Public: public},
	); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("NewTrustedKeys(duplicate id) error = %v, want ErrInvalid", err)
	}
	trust, err := NewTrustedKeys(TrustedKey{SignerKeyID: reviewerKeyID, Public: public})
	if err != nil {
		t.Fatalf("NewTrustedKeys() error = %v", err)
	}
	if _, err := Open(Config{Dir: t.TempDir(), Trust: trust,
		Signer: &Signer{SignerKeyID: "untrusted", Private: ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))},
	}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Open(untrusted signer) error = %v, want ErrUnsafe", err)
	}
}

func TestReadOnlyStoreCannotWrite(t *testing.T) {
	f := newFixture(t)
	readOnly, err := Open(Config{Dir: f.dir, Trust: f.trust, Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatalf("Open(read-only) error = %v", err)
	}
	_, err = readOnly.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	requireCategory(t, err, contracts.ErrUnsafe)
	_, err = readOnly.Revoke(context.Background(), RevokeRequest{Plan: f.plan, RevokedAt: f.now})
	requireCategory(t, err, contracts.ErrUnsafe)
	if _, err := os.Stat(f.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read-only store created %s", f.dir)
	}
}

func TestApproveRejectsInvalidMetadataAndPlan(t *testing.T) {
	f := newFixture(t)
	base := ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}
	cases := []struct {
		name   string
		mutate func(*ApproveRequest)
	}{
		{"empty reviewer", func(r *ApproveRequest) { r.ReviewerID = "" }},
		{"oversized reviewer", func(r *ApproveRequest) { r.ReviewerID = strings.Repeat("r", maxIdentityBytes+1) }},
		{"zero issued", func(r *ApproveRequest) { r.IssuedAt = time.Time{} }},
		{"empty window", func(r *ApproveRequest) { r.ExpiresAt = r.IssuedAt }},
		{"inverted window", func(r *ApproveRequest) { r.ExpiresAt = r.IssuedAt.Add(-time.Second) }},
		{"invalid plan", func(r *ApproveRequest) { r.Plan.ConfigDigest = "" }},
		{"plan without key", func(r *ApproveRequest) {
			r.Plan = clonePlan(r.Plan)
			for i := range r.Plan.Tables {
				for j := range r.Plan.Tables[i].Columns {
					r.Plan.Tables[i].Columns[j].KeyID = ""
				}
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			_, err := f.store.Approve(context.Background(), request)
			if !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("Approve() error = %v, want ErrInvalid", err)
			}
			if _, statErr := os.Stat(f.dir); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("rejected approval created %s", f.dir)
			}
		})
	}
}

func TestApproveRefusesAlreadyExpiredWindow(t *testing.T) {
	f := newFixture(t)
	_, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel,
		IssuedAt: f.now.Add(-2 * time.Hour), ExpiresAt: f.now,
	})
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errExpired)
}

func TestOperationsRejectBadContext(t *testing.T) {
	f := newFixture(t)
	if _, err := f.store.Lookup(nil, f.plan); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Lookup(nil ctx) error = %v, want ErrInvalid", err)
	}
	if _, err := f.store.Approve(nil, ApproveRequest{}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Approve(nil ctx) error = %v, want ErrInvalid", err)
	}
	if _, err := f.store.Revoke(nil, RevokeRequest{}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Revoke(nil ctx) error = %v, want ErrInvalid", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.store.Lookup(cancelled, f.plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("Lookup(cancelled ctx) error = %v, want context.Canceled", err)
	}
	f.approve(t)
	if _, err := f.store.Revoke(cancelled, RevokeRequest{Plan: f.plan, RevokedAt: f.now}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Revoke(cancelled ctx) error = %v, want context.Canceled", err)
	}
}

func TestLookupRejectsPathLikePlanID(t *testing.T) {
	f := newFixture(t)
	plan := clonePlan(f.plan)
	plan.ID = "../escaped"
	if _, err := f.lookup(plan); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Lookup(path plan id) error = %v, want ErrInvalid", err)
	}
	if _, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("Approve(path plan id) error = %v, want ErrInvalid", err)
	}
}

func TestReadOnlyFlowCreatesNoHistoryFile(t *testing.T) {
	f := newFixture(t)
	before := treeDigest(t, f.testRoot)

	readOnly, err := Open(Config{Dir: f.dir, Trust: f.trust})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := readOnly.Lookup(context.Background(), f.plan); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("Lookup() error = %v, want ErrNotFound", err)
	}
	other := clonePlan(f.plan)
	other.ID = "plan-digest-2"
	if _, err := readOnly.Lookup(context.Background(), other); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("Lookup(other) error = %v, want ErrNotFound", err)
	}
	if after := treeDigest(t, f.testRoot); after != before {
		t.Fatalf("read-only flow changed the tree: %s -> %s", before, after)
	}
	if _, err := os.Stat(f.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read-only flow created %s", f.dir)
	}

	// The same digest must be sensitive to a real write, otherwise the check above
	// would pass trivially.
	f.approve(t)
	if after := treeDigest(t, f.testRoot); after == before {
		t.Fatal("tree digest did not change after an explicit Approve")
	}
}

func TestStoredRecordContainsNoSecretMaterial(t *testing.T) {
	// The only secret the store ever holds is the review signing key; the store
	// takes no row values, primary-key values, masking keys or DSNs at all. This
	// test greps the serialised payload and the stored file for every form that
	// key material or a synthetic DSN could take.
	f := newFixture(t)
	approved := f.approve(t)
	path := filepath.Join(f.dir, f.plan.ID+recordSuffix)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	payload, err := approved.canonical()
	if err != nil {
		t.Fatalf("canonical() error = %v", err)
	}
	markers := [][]byte{
		[]byte("synthetic-dsn-marker-c1f4"),
		[]byte("synthetic-original-cell-marker-c1f4"),
		f.private,
		f.private.Seed(),
		[]byte(hex.EncodeToString(f.private)),
		[]byte(hex.EncodeToString(f.private.Seed())),
		[]byte(base64.StdEncoding.EncodeToString(f.private)),
		[]byte(base64.StdEncoding.EncodeToString(f.private.Seed())),
	}
	for _, marker := range markers {
		if bytes.Contains(raw, marker) {
			t.Fatal("stored record contains secret material")
		}
		if bytes.Contains(payload, marker) {
			t.Fatal("serialised record contains secret material")
		}
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != f.plan.ID+recordSuffix {
		t.Fatalf("store directory holds unexpected files: %v", entries)
	}
}

func TestCorruptOrTruncatedRecordRejected(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	path := filepath.Join(f.dir, f.plan.ID+recordSuffix)
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	payload := valid[bytes.IndexByte(valid, '\n')+1:]
	payload = payload[bytes.IndexByte(payload, '\n')+1:]
	cases := []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"magic only", []byte(magicLine)},
		{"magic and newline", []byte(magicLine + "\n")},
		{"no payload", []byte(magicLine + "\n" + hex.EncodeToString(make([]byte, ed25519.SignatureSize)) + "\n")},
		{"bad signature hex", []byte(magicLine + "\n" + "zz" + "\n" + string(payload))},
		{"short signature", []byte(magicLine + "\n" + "aabb" + "\n" + string(payload))},
		{"wrong magic", append([]byte("maskriver-other-v1\n"), valid[bytes.IndexByte(valid, '\n')+1:]...)},
		{"binary garbage", []byte{0x00, 0xff, 0x10, '\n', 0x01}},
		{"truncated half", valid[:len(valid)/2]},
		{"truncated payload tail", valid[:len(valid)-1]},
		{"truncated to magic", valid[:len(magicLine)]},
		{"non canonical payload", []byte(magicLine + "\n" +
			hex.EncodeToString(make([]byte, ed25519.SignatureSize)) + "\n" + `{"version":"history-record-v1"}`)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, test.body, storeFilePerm); err != nil {
				t.Fatalf("write corrupt record = %v", err)
			}
			_, err := f.lookup(f.plan)
			requireCategory(t, err, contracts.ErrUnsafe)
			if err := os.WriteFile(path, valid, storeFilePerm); err != nil {
				t.Fatalf("restore record = %v", err)
			}
			if _, err := f.lookup(f.plan); err != nil {
				t.Fatalf("restored record rejected: %v", err)
			}
		})
	}
}

func TestInterruptedWriteLeavesNoValidRecord(t *testing.T) {
	f := newFixture(t)
	f.store.rename = func(string, string) error { return errors.New("synthetic interruption") }
	_, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	requireCategory(t, err, contracts.ErrUnsafe)
	requireCause(t, err, errWrite)

	f.store.rename = os.Rename
	if _, err := f.lookup(f.plan); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("Lookup() after interrupted write error = %v, want ErrNotFound", err)
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("interrupted write left files behind: %v", entries)
	}
}

func TestFailedRenameKeepsPreviousRecordIntact(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	path := filepath.Join(f.dir, f.plan.ID+recordSuffix)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record = %v", err)
	}
	f.store.rename = func(string, string) error { return errors.New("synthetic interruption") }
	if _, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: f.plan, ReviewerID: "other@synthetic.example", IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("Approve() error = %v, want ErrUnsafe", err)
	}
	f.store.rename = os.Rename
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record after failed replace = %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed replace changed the stored record")
	}
	loaded, err := f.lookup(f.plan)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if loaded.ReviewerID != reviewerLabel {
		t.Fatalf("reviewer = %s, want the previous record", loaded.ReviewerID)
	}
}

func TestConcurrentWritersLeaveValidStore(t *testing.T) {
	f := newFixture(t)
	const writers = 8
	var wait sync.WaitGroup
	errs := make([]error, writers)
	wait.Add(writers)
	for index := 0; index < writers; index++ {
		go func(index int) {
			defer wait.Done()
			_, errs[index] = f.store.Approve(context.Background(), ApproveRequest{
				Plan:       f.plan,
				ReviewerID: reviewerLabel + "." + strconv.Itoa(index),
				IssuedAt:   f.now,
				ExpiresAt:  f.now.Add(time.Duration(index+1) * time.Hour),
			})
		}(index)
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Approve %d error = %v", index, err)
		}
	}
	if _, err := f.lookup(f.plan); err != nil {
		t.Fatalf("Lookup() after concurrent writes error = %v", err)
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("concurrent writes left %d entries, want 1: %v", len(entries), entries)
	}
}

func TestConcurrentWriterAndReaderNeverSeeTornRecord(t *testing.T) {
	f := newFixture(t)
	f.approve(t)
	var wait sync.WaitGroup
	wait.Add(2)
	readErr := make(chan error, 1)
	go func() {
		defer wait.Done()
		for attempt := 0; attempt < 20; attempt++ {
			if _, err := f.lookup(f.plan); err != nil {
				readErr <- err
				return
			}
		}
		readErr <- nil
	}()
	go func() {
		defer wait.Done()
		for attempt := 0; attempt < 20; attempt++ {
			if _, err := f.store.Approve(context.Background(), ApproveRequest{
				Plan: f.plan, ReviewerID: reviewerLabel, IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
			}); err != nil {
				return
			}
		}
	}()
	wait.Wait()
	if err := <-readErr; err != nil {
		t.Fatalf("reader observed an invalid store during concurrent rewrites: %v", err)
	}
}
