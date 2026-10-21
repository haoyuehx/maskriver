package runner

// Regression tests for work package C2: Apply consumes a read-only review
// decision and refuses every run that has no decision bound to exactly the plan.
// The tests here use the real history store for the authentication path and the
// testApproval double only in tests whose subject is a different gate.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	testReviewerKeyID = "synthetic-reviewer-1"
	testReviewerLabel = "reviewer@synthetic.example"
)

// testApproval is the read-only ApprovalQuery double for tests whose subject is
// another gate. It echoes the plan's own binding fields, so no refusal in those
// tests can come from the approval gate. The real store is exercised by every
// test in this file that names a decision.
type testApproval struct {
	calls int
	err   error
}

func (a *testApproval) Lookup(_ context.Context, plan contracts.Plan) (history.Record, error) {
	a.calls++
	if a.err != nil {
		return history.Record{}, a.err
	}
	record := history.Record{
		Version:          history.RecordVersion,
		PlanID:           plan.ID,
		SourceSnapshotID: plan.SourceSnapshotID,
		TargetID:         plan.TargetID,
		SchemaDigest:     plan.SchemaDigest,
		ConfigDigest:     plan.ConfigDigest,
		Scopes:           recordScopes(plan),
		IssuedAt:         time.Now().Add(-time.Minute),
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	// The record format binds one strategy pair and key for the whole plan, so
	// the double echoes the first planned column exactly like history.newRecord.
	if len(plan.Tables) != 0 && len(plan.Tables[0].Columns) != 0 {
		column := plan.Tables[0].Columns[0]
		record.StrategyID = column.Strategy.ID
		record.StrategyVersion = column.Strategy.Version
		record.KeyID = column.KeyID
	}
	return record, nil
}

func recordScopes(plan contracts.Plan) []history.Scope {
	var scopes []history.Scope
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			scopes = append(scopes, history.Scope{
				Table: table.Table, Column: column.Column.Column,
			})
		}
	}
	return scopes
}

func useTestApproval(e *Engine) { e.SetApprovalQuery(&testApproval{}) }

// approvalFunc adapts a function to the ApprovalQuery seam.
type approvalFunc func(context.Context, contracts.Plan) (history.Record, error)

func (f approvalFunc) Lookup(ctx context.Context, plan contracts.Plan) (history.Record, error) {
	return f(ctx, plan)
}

// reviewer holds one synthetic reviewer key and the trust set naming it under
// testReviewerKeyID.
type reviewer struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newReviewer(t *testing.T, seed byte) reviewer {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatal("ed25519 private key did not expose a public key")
	}
	return reviewer{private: private, public: public}
}

func (r reviewer) trust(t *testing.T) history.TrustedKeys {
	t.Helper()
	trust, err := history.NewTrustedKeys(history.TrustedKey{SignerKeyID: testReviewerKeyID, Public: r.public})
	if err != nil {
		t.Fatalf("NewTrustedKeys() error = %v", err)
	}
	return trust
}

// issue stores one real decision for the whole plan and returns the store and its
// directory.
func (r reviewer) issue(t *testing.T, plan contracts.Plan, now time.Time) (*history.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "history")
	store, err := history.Open(history.Config{
		Dir: dir, Trust: r.trust(t),
		Signer: &history.Signer{SignerKeyID: testReviewerKeyID, Private: r.private},
		Now:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("history.Open() error = %v", err)
	}
	if _, err := store.Approve(context.Background(), history.ApproveRequest{
		Plan: plan, ReviewerID: testReviewerLabel,
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	return store, dir
}

// readOnly opens the same store directory without a signer, which is the shape
// Apply receives.
func (r reviewer) readOnly(t *testing.T, dir string, now func() time.Time) *history.Store {
	t.Helper()
	store, err := history.Open(history.Config{Dir: dir, Trust: r.trust(t), Now: now})
	if err != nil {
		t.Fatalf("history.Open(read-only) error = %v", err)
	}
	return store
}

// assertApplyRefusedBeforeWriter requires the run to fail with want (nil means
// any error) without any Writer capability and without changing the target tree.
func assertApplyRefusedBeforeWriter(t *testing.T, e *Engine, plan contracts.Plan, want error) {
	t.Helper()
	before := acceptanceTreeDigest(t, e.cfg.Target)
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: 2,
	})
	if err == nil {
		t.Fatalf("Apply() succeeded, want a refusal; report=%+v", report)
	}
	if want != nil && !errors.Is(err, want) {
		t.Fatalf("Apply() error = %v, want %v", err, want)
	}
	if report.Complete || len(report.Committed) != 0 || report.CommitOutcomeUnknown {
		t.Fatalf("refused apply reported work: %+v", report)
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("refused apply used the Writer: opens=%d begins=%d updates=%d commits=%d",
			spy.opens, spy.begins, spy.updates, spy.commits)
	}
	if after := acceptanceTreeDigest(t, e.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("refused apply changed the target fixture tree")
	}
}

// twoTableApprovalEngine opens the two-table fixture with both scopes declared.
func twoTableApprovalEngine(t *testing.T) *Engine {
	t.Helper()
	cfg := twoTableFixture(t)
	cfg.Table, cfg.Column = "", ""
	cfg.Scopes = twoTableScopes()
	return scopeEngine(t, cfg)
}

func TestApplyRequiresApprovalQuery(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	plan := mustPlan(t, e)
	e.SetApprovalQuery(nil)
	assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
}

func TestApplyAcceptsAuthenticatedDecisionForWholePlan(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	ctx := context.Background()
	plan := mustPlan(t, e)
	store, _ := newReviewer(t, 0x51).issue(t, plan, time.Now())
	e.SetApprovalQuery(store)
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if err != nil || !report.Complete || len(report.Committed) != 2 {
		t.Fatalf("authenticated apply report=%+v error=%v", report, err)
	}
	validation, err := e.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil {
		t.Fatalf("strict validate error = %v", err)
	}
	if !validation.Passed(true) {
		t.Fatalf("strict validate did not pass after authenticated apply: issues=%+v coverage=%+v",
			validation.Issues, validation.Coverage)
	}
}

func TestApplyRefusesEveryUnauthenticatedDecision(t *testing.T) {
	twoRows := []string{"a@example.invalid", "b@example.invalid"}

	t.Run("no record", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		store := newReviewer(t, 0x51).readOnly(t, filepath.Join(t.TempDir(), "history"), nil)
		e.SetApprovalQuery(store)
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrNotFound)
	})
	t.Run("tampered record", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		_, dir := reviewer.issue(t, plan, time.Now())
		path := filepath.Join(dir, plan.ID+".decision")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)/2] ^= 0x01
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		e.SetApprovalQuery(reviewer.readOnly(t, dir, nil))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("forged signature", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		attacker := newReviewer(t, 0x66)
		if _, dir := attacker.issue(t, plan, time.Now()); dir == "" {
			t.Fatal("attacker fixture has no store")
		} else {
			// The deployment trust set names only the official reviewer, so the
			// attacker-signed record is refused as an unknown signer.
			e.SetApprovalQuery(newReviewer(t, 0x51).readOnly(t, dir, nil))
		}
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("expired record", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		issuedAt := time.Now()
		reviewer := newReviewer(t, 0x51)
		_, dir := reviewer.issue(t, plan, issuedAt)
		e.SetApprovalQuery(reviewer.readOnly(t, dir, func() time.Time { return issuedAt.Add(2 * time.Hour) }))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("revoked record", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		now := time.Now()
		store, _ := reviewer.issue(t, plan, now)
		if _, err := store.Revoke(context.Background(), history.RevokeRequest{
			Plan: plan, RevokedAt: now.Add(time.Minute),
		}); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		e.SetApprovalQuery(store)
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("future-issued record", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		future := time.Now().Add(time.Hour)
		store, err := history.Open(history.Config{
			Dir: filepath.Join(t.TempDir(), "history"), Trust: reviewer.trust(t),
			Signer: &history.Signer{SignerKeyID: testReviewerKeyID, Private: reviewer.private},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Approve(context.Background(), history.ApproveRequest{
			Plan: plan, ReviewerID: testReviewerLabel,
			IssuedAt: future, ExpiresAt: future.Add(time.Hour),
		}); err != nil {
			t.Fatalf("Approve(future window) error = %v", err)
		}
		e.SetApprovalQuery(store)
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("unreadable store", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		dir := filepath.Join(t.TempDir(), "history")
		if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		e.SetApprovalQuery(newReviewer(t, 0x51).readOnly(t, dir, nil))
		// The required property is refusal before any Writer, not the exact
		// error category: a path that is a regular file maps to a different
		// category per platform (Windows reports "object not found", POSIX a
		// store error), and both are still a refusal.
		assertApplyRefusedBeforeWriter(t, e, plan, nil)
	})
	t.Run("store without trusted keys", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		e.SetApprovalQuery(approvalFunc(func(context.Context, contracts.Plan) (history.Record, error) {
			_, err := history.Open(history.Config{Dir: t.TempDir()})
			return history.Record{}, err
		}))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("decision for another plan", func(t *testing.T) {
		e, _ := acceptanceEngine(t, twoRows)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		store, _ := reviewer.issue(t, plan, time.Now())
		e.SetApprovalQuery(approvalFunc(func(ctx context.Context, requested contracts.Plan) (history.Record, error) {
			record, err := store.Lookup(ctx, requested)
			record.PlanID = "other-plan"
			return record, err
		}))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("one table of a two-table plan", func(t *testing.T) {
		e := twoTableApprovalEngine(t)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		store, _ := reviewer.issue(t, plan, time.Now())
		e.SetApprovalQuery(approvalFunc(func(ctx context.Context, requested contracts.Plan) (history.Record, error) {
			record, err := store.Lookup(ctx, requested)
			if err == nil && len(record.Scopes) > 1 {
				record.Scopes = record.Scopes[:1]
			}
			return record, err
		}))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("reordered scopes", func(t *testing.T) {
		e := twoTableApprovalEngine(t)
		plan := mustPlan(t, e)
		reviewer := newReviewer(t, 0x51)
		store, _ := reviewer.issue(t, plan, time.Now())
		e.SetApprovalQuery(approvalFunc(func(ctx context.Context, requested contracts.Plan) (history.Record, error) {
			record, err := store.Lookup(ctx, requested)
			if err == nil && len(record.Scopes) > 1 {
				record.Scopes[0], record.Scopes[1] = record.Scopes[1], record.Scopes[0]
			}
			return record, err
		}))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
}

// nilApproval dereferences its receiver, so a typed-nil instance would panic if
// Apply ever called Lookup on it instead of failing closed.
type nilApproval struct{ calls int }

func (q *nilApproval) Lookup(context.Context, contracts.Plan) (history.Record, error) {
	q.calls++
	return history.Record{}, nil
}

// TestApplyRefusesUnusableApprovalSeam requires every abnormal dependency state
// - a nil interface, a typed-nil pointer, a nil function seam and a seam that
// fails with an unknown error - to refuse the run with contracts.ErrUnsafe
// before any Writer exists. A typed-nil pointer used to panic inside Lookup.
func TestApplyRefusesUnusableApprovalSeam(t *testing.T) {
	t.Run("typed-nil pointer", func(t *testing.T) {
		e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
		plan := mustPlan(t, e)
		var query *nilApproval
		e.SetApprovalQuery(query)
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("nil function", func(t *testing.T) {
		e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
		plan := mustPlan(t, e)
		var seam approvalFunc
		e.SetApprovalQuery(seam)
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
	t.Run("unknown seam failure", func(t *testing.T) {
		e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
		plan := mustPlan(t, e)
		e.SetApprovalQuery(approvalFunc(func(context.Context, contracts.Plan) (history.Record, error) {
			return history.Record{}, errors.New("approval seam unavailable")
		}))
		assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
	})
}

// TestApplyRefusesRecordFailingRunnerRecheck simulates a buggy Lookup seam that
// returns a record the store would never have released: wrong record version,
// expired, issued in the future, revoked after the lookup, or bound to another
// strategy pair or key. The engine must re-derive those fields itself instead of
// trusting the seam, and refuse before a Writer exists.
func TestApplyRefusesRecordFailingRunnerRecheck(t *testing.T) {
	revokedAt := time.Now()
	cases := []struct {
		name   string
		mutate func(*history.Record)
	}{
		{"wrong record version", func(r *history.Record) { r.Version = "history-record-v0" }},
		{"expired record", func(r *history.Record) { r.ExpiresAt = time.Now().Add(-time.Minute) }},
		{"future-issued record", func(r *history.Record) { r.IssuedAt = time.Now().Add(time.Hour) }},
		{"revoked after lookup", func(r *history.Record) { r.RevokedAt = &revokedAt }},
		{"strategy id", func(r *history.Record) { r.StrategyID = "other-strategy" }},
		{"strategy version", func(r *history.Record) { r.StrategyVersion = "99" }},
		{"key id", func(r *history.Record) { r.KeyID = "other-key" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
			plan := mustPlan(t, e)
			store, _ := newReviewer(t, 0x51).issue(t, plan, time.Now())
			e.SetApprovalQuery(approvalFunc(func(ctx context.Context, requested contracts.Plan) (history.Record, error) {
				record, err := store.Lookup(ctx, requested)
				if err != nil {
					return record, err
				}
				tc.mutate(&record)
				return record, nil
			}))
			assertApplyRefusedBeforeWriter(t, e, plan, contracts.ErrUnsafe)
		})
	}
}

// TestReadOnlyWorkflowsNeverTouchApprovalStore proves Scan, Plan, Preview and
// Validate need no decision and can not create or modify a history file, even
// when a store directory is injected.
func TestReadOnlyWorkflowsNeverTouchApprovalStore(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	ctx := context.Background()
	plan := mustPlan(t, e)

	counter := &testApproval{}
	e.SetApprovalQuery(counter)
	if _, err := e.Scan(ctx, contracts.ScanRequest{}); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if _, err := e.Plan(ctx); err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if _, err := e.Preview(ctx, contracts.PreviewRequest{Plan: plan}); err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if _, err := e.Validate(ctx, contracts.ValidateRequest{Plan: plan}); err != nil && !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("Validate() error = %v", err)
	}
	if counter.calls != 0 {
		t.Fatalf("read-only workflows called Lookup %d times", counter.calls)
	}

	dir := filepath.Join(t.TempDir(), "history")
	e.SetApprovalQuery(newReviewer(t, 0x51).readOnly(t, dir, nil))
	if _, err := e.Preview(ctx, contracts.PreviewRequest{Plan: plan}); err != nil {
		t.Fatalf("Preview() with a store error = %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only preview created the history store at %s", dir)
	}
}

// TestApplyReportingGatesStillFireWithValidDecision keeps the partial-commit and
// unknown-commit semantics observable after an authenticated decision, so the
// new gate can not hide or reinterpret durable work.
func TestApplyReportingGatesStillFireWithValidDecision(t *testing.T) {
	t.Run("commit outcome unknown", func(t *testing.T) {
		e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
		plan := mustPlan(t, e)
		store, _ := newReviewer(t, 0x51).issue(t, plan, time.Now())
		e.SetApprovalQuery(store)
		spy := &writerSpy{commitErr: fmt.Errorf("driver status: %w", contracts.ErrCommitUnknown), commitErrAt: 2}
		spyEngine(t, e, spy)
		report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
		if !errors.Is(err, contracts.ErrCommitUnknown) || !report.CommitOutcomeUnknown || report.Complete {
			t.Fatalf("unknown commit result report=%+v error=%v", report, err)
		}
		if len(report.Committed) != 1 || spy.rollbacks != 0 {
			t.Fatalf("unknown commit accounting report=%+v rollbacks=%d", report, spy.rollbacks)
		}
	})
	t.Run("partial commit", func(t *testing.T) {
		e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
		plan := mustPlan(t, e)
		store, _ := newReviewer(t, 0x51).issue(t, plan, time.Now())
		e.SetApprovalQuery(store)
		spy := &writerSpy{commitErr: contracts.ErrDatabase, commitErrAt: 2}
		spyEngine(t, e, spy)
		report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
		if err == nil || report.Complete || report.CommitOutcomeUnknown {
			t.Fatalf("partial commit report=%+v error=%v", report, err)
		}
		if len(report.Committed) != 1 || report.Committed[0].Ordinal != 1 {
			t.Fatalf("earlier committed receipt lost: %+v", report.Committed)
		}
	})
}

func TestRecordAuthorizesToleratesSmallFutureIssuedSkew(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	plan := mustPlan(t, e)
	record, _ := (&testApproval{}).Lookup(context.Background(), plan)
	now := time.Now()
	record.IssuedAt = now.Add(time.Minute)
	if !recordAuthorizes(record, plan, now) {
		t.Fatal("+1 minute future-issued record should be tolerated")
	}
	record.IssuedAt = now.Add(6 * time.Minute)
	if recordAuthorizes(record, plan, now) {
		t.Fatal("+6 minute future-issued record should be refused")
	}
}
