package history

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func streamingStore(t *testing.T, dir string) (*Store, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := NewTrustedKeys(TrustedKey{SignerKeyID: "reviewer-1", Public: public})
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(Config{
		Dir: dir, Trust: trust,
		Signer: &Signer{SignerKeyID: "reviewer-1", Private: private},
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, private
}

func streamingPlan(t *testing.T) contracts.Plan {
	t.Helper()
	ref := contracts.TableRef{Database: "maskriver", Schema: "main", Table: "people"}
	col := contracts.ColumnRef{TableRef: ref, Column: "email"}
	plan := contracts.Plan{
		Revision: contracts.APIRevision, SourceSnapshotID: strings.Repeat("a", 64),
		TargetID: strings.Repeat("b", 64), SchemaDigest: strings.Repeat("c", 64),
		ConfigDigest: strings.Repeat("d", 64), ScanComplete: true,
		Tables: []contracts.TablePlan{{
			Table: ref, PrimaryKey: []string{"id"},
			Columns: []contracts.ColumnPlan{{
				Column: col, Type: contracts.Type{Kind: contracts.Text, Native: "TEXT", Nullable: true},
				Strategy: contracts.StrategyRef{ID: "redact", Version: "1"},
				Scope:    "sqlite/people/email", NormalizationVersion: "typed-lexical-v1", KeyID: "synthetic",
			}},
		}},
	}
	plan.ID = planIDForTest(plan)
	if err := plan.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func planIDForTest(plan contracts.Plan) string {
	sum := sha256.Sum256([]byte(plan.Revision + plan.SourceSnapshotID + plan.TargetID +
		plan.SchemaDigest + plan.ConfigDigest))
	return hex.EncodeToString(sum[:])
}

func TestStreamingApproveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	record, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("approve streaming: %v", err)
	}
	if record.Protocol != contracts.StreamingProtocolRevision ||
		record.BindingRevision != contracts.ApprovalBindingRevision {
		t.Fatalf("record protocol/binding = %q/%q", record.Protocol, record.BindingRevision)
	}
	if _, err := store.Lookup(context.Background(), plan); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("legacy lookup found a streaming record: %v", err)
	}
	approval, err := store.LoadStreamingApproval(context.Background(), plan)
	if err != nil {
		t.Fatalf("load approval: %v", err)
	}
	if _, err := store.VerifyStreaming(context.Background(), plan, approval); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, plan.ID+streamingSuffix)); err != nil {
		t.Fatalf("streaming file missing: %v", err)
	}
}

func TestStreamingRecordDoesNotAuthorizeLegacyAndViceVersa(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.Approve(context.Background(), ApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStreamingApproval(context.Background(), plan); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("legacy approval authorized streaming: %v", err)
	}
	dir2 := t.TempDir()
	store2, _ := streamingStore(t, dir2)
	if _, err := store2.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store2.Lookup(context.Background(), plan); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("streaming approval authorized legacy: %v", err)
	}
}

func TestStreamingApprovalRejectsContentDrift(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*contracts.Plan){
		"scope": func(p *contracts.Plan) {
			p.Tables[0].Columns[0].Scope = "sqlite/people/other"
		},
		"normalization": func(p *contracts.Plan) {
			p.Tables[0].Columns[0].NormalizationVersion = "typed-lexical-v2"
		},
		"type": func(p *contracts.Plan) {
			p.Tables[0].Columns[0].Type.Length = 10
		},
		"primary-key": func(p *contracts.Plan) { p.Tables[0].PrimaryKey = []string{"email"} },
		"scopes-reordered": func(p *contracts.Plan) {
			ref := contracts.TableRef{Database: "maskriver", Schema: "main", Table: "other"}
			p.Tables = append(p.Tables, contracts.TablePlan{
				Table: ref, PrimaryKey: []string{"id"},
				Columns: []contracts.ColumnPlan{{
					Column:   contracts.ColumnRef{TableRef: ref, Column: "email"},
					Type:     contracts.Type{Kind: contracts.Text, Native: "TEXT", Nullable: true},
					Strategy: contracts.StrategyRef{ID: "redact", Version: "1"},
					Scope:    "sqlite/other/email", NormalizationVersion: "typed-lexical-v1", KeyID: "synthetic",
				}},
			})
			p.Tables[0], p.Tables[1] = p.Tables[1], p.Tables[0]
			p.ID = planIDForTest(*p)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := plan
			changed.Tables = append([]contracts.TablePlan(nil), plan.Tables...)
			for i := range changed.Tables {
				changed.Tables[i].Columns = append([]contracts.ColumnPlan(nil), plan.Tables[i].Columns...)
			}
			mutate(&changed)
			if _, err := store.LoadStreamingApproval(context.Background(), changed); !errors.Is(err, contracts.ErrConflict) &&
				!errors.Is(err, contracts.ErrInvalid) && !errors.Is(err, contracts.ErrUnsafe) {
				t.Fatalf("drift %q accepted: %v", name, err)
			}
		})
	}
}

func TestStreamingApprovalRevocationAndClock(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeStreaming(context.Background(), plan, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStreamingApproval(context.Background(), plan); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("revoked approval loaded: %v", err)
	}
	// Expired.
	dir2 := t.TempDir()
	store2, _ := streamingStore(t, dir2)
	plan2 := streamingPlan(t)
	plan2.ID = strings.Repeat("e", 64)
	_ = plan2
	if _, err := store2.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan2, ReviewerID: "reviewer-a", IssuedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
	}); err == nil {
		t.Fatal("approve accepted an already-expired window")
	}
}

func TestStreamingApprovalUnknownSignerAndTamper(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, plan.ID+streamingSuffix)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), raw...)
	for i := range tampered {
		if tampered[i] == 'a' {
			tampered[i] = 'b'
			break
		}
	}
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStreamingApproval(context.Background(), plan); err == nil {
		t.Fatal("tampered record loaded")
	}
}

func TestStreamingDuplicateRecordAmbiguous(t *testing.T) {
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, plan.ID+streamingSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"+streamingSuffix), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStreamingApproval(context.Background(), plan); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("ambiguous store accepted: %v", err)
	}
}

func TestBoundedHistoryEnumeration(t *testing.T) {
	dir := t.TempDir()
	public, private, _ := ed25519.GenerateKey(nil)
	trust, _ := NewTrustedKeys(TrustedKey{SignerKeyID: "reviewer-1", Public: public})
	store, err := Open(Config{
		Dir: dir, Trust: trust, MaxEntries: 4,
		Signer: &Signer{SignerKeyID: "reviewer-1", Private: private},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(dir, "noise-"+string(rune('a'+i))+".txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.LoadStreamingApproval(context.Background(), streamingPlan(t)); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("entry bound not enforced: %v", err)
	}
	// Context cancellation ends the enumeration.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LoadStreamingApproval(ctx, streamingPlan(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not honoured: %v", err)
	}
}

func TestLegacyLookupKeepsLargeDirectoryBehaviour(t *testing.T) {
	dir := t.TempDir()
	public, private, _ := ed25519.GenerateKey(nil)
	trust, _ := NewTrustedKeys(TrustedKey{SignerKeyID: "reviewer-1", Public: public})
	store, err := Open(Config{
		Dir: dir, Trust: trust, MaxEntries: 4,
		Signer: &Signer{SignerKeyID: "reviewer-1", Private: private},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err := os.WriteFile(filepath.Join(dir, "noise-"+string(rune('a'+i))+".txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := streamingPlan(t)
	now := time.Now()
	// The legacy path keeps its old behaviour: the streaming entry cap does not
	// reject a pre-existing large M1 directory.
	if _, err := store.Approve(context.Background(), ApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("legacy approve in large directory: %v", err)
	}
	if _, err := store.Lookup(context.Background(), plan); err != nil {
		t.Fatalf("legacy lookup in large directory: %v", err)
	}
}

func TestStreamingUnreadableAmbiguityFailsClosed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("mode-000 files are not meaningful on this platform")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read mode-000 files")
	}
	dir := t.TempDir()
	store, _ := streamingStore(t, dir)
	plan := streamingPlan(t)
	now := time.Now()
	if _, err := store.ApproveStreaming(context.Background(), StreamingApproveRequest{
		Plan: plan, ReviewerID: "reviewer-a", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(dir, "other"+streamingSuffix)
	if err := os.WriteFile(candidate, []byte("unreadable"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStreamingApproval(context.Background(), plan); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("unreadable candidate did not fail closed: %v", err)
	}
}
