package runner

// Regression for #9: a plan whose replacement for a non-NULL sensitive value is
// NULL can never pass validation, because verify reports a NULL target value
// for a non-NULL source value as conflicting evidence. Apply's dry run computes
// every replacement before a Writer exists, so this plan must be refused there
// instead of being written and rejected by a later strict validate.

import (
	"context"
	"errors"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestApplyRefusesNullReplacementBeforeWriter(t *testing.T) {
	// email is deliberately NULL-able: on a NOT NULL column the null strategy
	// already refuses, so only this fixture exercises a replacement that the
	// strategy is willing to emit and validation can never accept.
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
	)
	cfg.Table, cfg.Column = "people", "email"
	cfg.Strategy = contracts.StrategyRef{ID: "null", Version: "1"}
	engine := scopeEngine(t, cfg)
	// The plan is buildable, so the refusal under test is the write gate and not
	// the evidence gates in Plan.
	plan := mustPlan(t, engine)
	assertApplyRefused(t, engine, plan, contracts.ErrUnsafe)
	// Preview is Apply's dry run: the same refusal, still before any row is read
	// for writing.
	if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("preview error=%v, want ErrUnsafe", err)
	}
}
