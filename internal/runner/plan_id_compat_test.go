package runner

import (
	"context"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
)

// These digests were measured from the pre-multi-table implementation at
// a1cdcc262d57973c63e991ca3c28210b0fcaf379 using the same fixture, not from the
// current code. They pin backward compatibility across the multi-table change:
// regenerating them from the current implementation would make the test
// vacuous.
const (
	legacySingleScopeSchemaDigest = "6fa1f0d14f2b0cbef8a9432ae7f70c0b779af3c9490482eec85f7a6c5c6a7132"
	legacySingleScopeConfigDigest = "02a1c4efb08ed0fe3061a3008c180866d57321c4ca3bffe2bd900cae66b03659"
)

// TestSingleScopeDigestsMatchLegacyEncoding is the cross-version golden
// regression: a single-scope plan must hash exactly as it did before the
// multi-table change, for both the implicit -table/-column path and an explicit
// single -scope.
func TestSingleScopeDigestsMatchLegacyEncoding(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})

	t.Run("implicit single table and column", func(t *testing.T) {
		plan, err := e.Plan(context.Background())
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.SchemaDigest != legacySingleScopeSchemaDigest {
			t.Fatalf("SchemaDigest=%s, want legacy %s", plan.SchemaDigest, legacySingleScopeSchemaDigest)
		}
		if plan.ConfigDigest != legacySingleScopeConfigDigest {
			t.Fatalf("ConfigDigest=%s, want legacy %s", plan.ConfigDigest, legacySingleScopeConfigDigest)
		}
		// The ID formula itself must be unchanged: same inputs, same ID.
		want := hashString(plan.Revision + plan.SourceSnapshotID + plan.TargetID +
			plan.SchemaDigest + plan.ConfigDigest)
		if plan.ID != want {
			t.Fatalf("PlanID=%s, want %s", plan.ID, want)
		}
	})

	t.Run("explicit single scope", func(t *testing.T) {
		e.cfg.Scopes = e.cfg.ResolvedScopes()
		e.cfg.Table, e.cfg.Column = "", ""
		plan, err := e.Plan(context.Background())
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.SchemaDigest != legacySingleScopeSchemaDigest {
			t.Fatalf("explicit scope SchemaDigest=%s, want legacy %s",
				plan.SchemaDigest, legacySingleScopeSchemaDigest)
		}
		if plan.ConfigDigest != legacySingleScopeConfigDigest {
			t.Fatalf("explicit scope ConfigDigest=%s, want legacy %s",
				plan.ConfigDigest, legacySingleScopeConfigDigest)
		}
		if len(plan.Tables) != 1 || len(plan.Tables[0].Columns) != 1 {
			t.Fatalf("explicit single scope produced %d tables", len(plan.Tables))
		}
	})
}

// TestMultiScopeSchemaDigestIsBoundToTableOrder guards the other direction: a
// multi-table plan must bind the ordered schema slice, so reordering changes the
// digest and identical order is reproducible.
func TestMultiScopeSchemaDigestIsBoundToTableOrder(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Table, cfg.Column = "", ""
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	e := scopeEngine(t, cfg)
	forward, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("forward plan: %v", err)
	}
	again, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("repeat plan: %v", err)
	}
	if forward.SchemaDigest != again.SchemaDigest {
		t.Fatalf("schema digest not reproducible: %s vs %s", forward.SchemaDigest, again.SchemaDigest)
	}
	if forward.SchemaDigest == legacySingleScopeSchemaDigest {
		t.Fatal("multi-table plan reused the single-object schema encoding")
	}

	e.cfg.Scopes = []config.Scope{e.cfg.Scopes[1], e.cfg.Scopes[0]}
	reversed, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("reversed plan: %v", err)
	}
	if reversed.SchemaDigest == forward.SchemaDigest {
		t.Fatal("reordering tables did not change SchemaDigest")
	}
	if reversed.ID == forward.ID {
		t.Fatal("reordering tables did not change Plan.ID")
	}
}
