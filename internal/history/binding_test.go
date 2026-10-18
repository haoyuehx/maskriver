package history

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestLookupRejectsPartialMultiTableApproval proves that a correctly signed
// record still fails when it covers only part of a multi-table plan: the
// signature authenticates the record, the binding check decides whether it
// authorises this plan.
func TestLookupRejectsPartialMultiTableApproval(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Record)
	}{
		{"first table only", func(r *Record) { r.Scopes = r.Scopes[:1] }},
		{"second table only", func(r *Record) { r.Scopes = r.Scopes[1:] }},
		{"missing last scope", func(r *Record) { r.Scopes = r.Scopes[:len(r.Scopes)-1] }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.writeRecord(t, f.plan.ID, f.buildRecord(t, test.mutate), f.private)
			_, err := f.lookup(f.plan)
			requireCategory(t, err, contracts.ErrConflict)
			requireCause(t, err, errBinding)
		})
	}
	t.Run("empty scope list", func(t *testing.T) {
		// A record with no scope cannot even be decoded as a record.
		f := newFixture(t)
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) { r.Scopes = nil }), f.private)
		_, err := f.lookup(f.plan)
		requireCategory(t, err, contracts.ErrUnsafe)
		requireCause(t, err, errCorrupt)
	})
}

// TestLookupRejectsScopeReorder covers both directions: a record whose scope
// order differs from the plan, and a plan whose scope order differs from the
// record.
func TestLookupRejectsScopeReorder(t *testing.T) {
	t.Run("record reordered", func(t *testing.T) {
		f := newFixture(t)
		f.writeRecord(t, f.plan.ID, f.buildRecord(t, func(r *Record) {
			r.Scopes[0], r.Scopes[1] = r.Scopes[1], r.Scopes[0]
		}), f.private)
		_, err := f.lookup(f.plan)
		requireCategory(t, err, contracts.ErrConflict)
		requireCause(t, err, errBinding)
	})
	t.Run("plan tables reordered", func(t *testing.T) {
		f := newFixture(t)
		f.approve(t)
		reordered := clonePlan(f.plan)
		reordered.Tables[0], reordered.Tables[1] = reordered.Tables[1], reordered.Tables[0]
		_, err := f.lookup(reordered)
		requireCategory(t, err, contracts.ErrConflict)
	})
	t.Run("plan columns reordered", func(t *testing.T) {
		f := newFixture(t)
		f.approve(t)
		reordered := clonePlan(f.plan)
		columns := reordered.Tables[1].Columns
		columns[0], columns[1] = columns[1], columns[0]
		_, err := f.lookup(reordered)
		requireCategory(t, err, contracts.ErrConflict)
	})
}

// TestLookupRejectsChangedBoundField covers every field the record binds. The
// record side is signed by the trusted key, so the refusal has to come from the
// binding check rather than from the signature.
func TestLookupRejectsChangedBoundField(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Record)
	}{
		{"plan id", func(r *Record) { r.PlanID = "plan-digest-2" }},
		{"source snapshot", func(r *Record) { r.SourceSnapshotID = "source-digest-2" }},
		{"target id", func(r *Record) { r.TargetID = "target-digest-2" }},
		{"schema digest", func(r *Record) { r.SchemaDigest = "schema-digest-2" }},
		{"config digest", func(r *Record) { r.ConfigDigest = "config-digest-2" }},
		{"strategy id", func(r *Record) { r.StrategyID = "phone" }},
		{"strategy version", func(r *Record) { r.StrategyVersion = "2" }},
		{"key id", func(r *Record) { r.KeyID = "key-2" }},
		{"table identity", func(r *Record) { r.Scopes[0].Table.Schema = "other" }},
		{"column name", func(r *Record) { r.Scopes[2].Column = "other" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.writeRecord(t, f.plan.ID, f.buildRecord(t, test.mutate), f.private)
			_, err := f.lookup(f.plan)
			requireCategory(t, err, contracts.ErrConflict)
			requireCause(t, err, errBinding)
		})
	}
}

// TestLookupRejectsLivePlanChange is the other direction: the stored record is
// untouched and the live plan changes while keeping the same Plan.ID, which is
// exactly what a tampered or stale plan looks like.
func TestLookupRejectsLivePlanChange(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contracts.Plan)
	}{
		{"source snapshot", func(p *contracts.Plan) { p.SourceSnapshotID = "source-digest-2" }},
		{"target id", func(p *contracts.Plan) { p.TargetID = "target-digest-2" }},
		{"schema digest", func(p *contracts.Plan) { p.SchemaDigest = "schema-digest-2" }},
		{"config digest", func(p *contracts.Plan) { p.ConfigDigest = "config-digest-2" }},
		{"strategy version", func(p *contracts.Plan) {
			for index := range p.Tables {
				for column := range p.Tables[index].Columns {
					p.Tables[index].Columns[column].Strategy.Version = "2"
				}
			}
		}},
		{"key id", func(p *contracts.Plan) {
			for index := range p.Tables {
				for column := range p.Tables[index].Columns {
					p.Tables[index].Columns[column].KeyID = "key-2"
				}
			}
		}},
		{"extra column", func(p *contracts.Plan) {
			table := p.Tables[0]
			p.Tables[0].Columns = append(p.Tables[0].Columns,
				contracts.ColumnPlan{
					Column:               contracts.ColumnRef{TableRef: table.Table, Column: "extra"},
					Type:                 contracts.Type{Kind: contracts.Text},
					Strategy:             contracts.StrategyRef{ID: "email", Version: "1"},
					Scope:                "sqlite/people/extra",
					NormalizationVersion: "typed-lexical-v1",
					KeyID:                "key-1",
				})
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.approve(t)
			changed := clonePlan(f.plan)
			test.mutate(&changed)
			if changed.ID != f.plan.ID {
				t.Fatalf("test plan changed the plan id, which would only exercise a missing file")
			}
			_, err := f.lookup(changed)
			requireCategory(t, err, contracts.ErrConflict)
			requireCause(t, err, errBinding)
		})
	}
}

// TestLookupRejectsMixedStrategyPlan shows that a plan whose columns disagree
// about strategy or key has no single binding, so Approve refuses it instead of
// picking whichever column happened to come first.
func TestLookupRejectsMixedStrategyPlan(t *testing.T) {
	f := newFixture(t)
	mixed := clonePlan(f.plan)
	mixed.Tables[1].Columns[1].Strategy.Version = "2"
	_, err := f.store.Approve(context.Background(), ApproveRequest{
		Plan: mixed, ReviewerID: reviewerLabel,
		IssuedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	requireCategory(t, err, contracts.ErrConflict)
	_, statErr := f.lookup(f.plan)
	requireCategory(t, statErr, contracts.ErrNotFound)
}

// TestLookupRejectsRecordForAnotherPlan stores a validly signed record under the
// requested plan's file name; the record still names its own plan, so it cannot
// authorise this one.
func TestLookupRejectsRecordForAnotherPlan(t *testing.T) {
	f := newFixture(t)
	other := clonePlan(f.plan)
	other.ID = "plan-digest-2"
	record, err := newRecord(other, reviewerLabel, f.now, f.now.Add(time.Hour), f.keyID)
	if err != nil {
		t.Fatalf("newRecord() error = %v", err)
	}
	f.writeRecord(t, f.plan.ID, record, f.private)
	if _, err := f.lookup(f.plan); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("Lookup() error = %v, want ErrConflict", err)
	}
}
