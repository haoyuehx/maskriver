package runner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// The scan gate must be independent of approval: `cfg.Approved` is an extra
// intent signal from the operator, never a substitute for detection evidence.
// A stored reviewer decision (work package C2) authenticates a plan that already
// carries complete Sensitive evidence; it can not turn an UNKNOWN or truncated
// scan into an applicable plan, because such a scan yields no plan at all.

func TestPlanScansSensitiveColumnEvenWhenApproved(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	if !e.cfg.Approved {
		t.Fatal("fixture must exercise the approved path")
	}
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("sensitive column plan error=%v", err)
	}
	if !plan.ScanComplete {
		t.Fatal("plan claims an incomplete scan for sensitive evidence")
	}
	if len(plan.Unresolved) != 0 {
		t.Fatalf("resolved plan has unresolved columns: %+v", plan.Unresolved)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil || !report.Complete {
		t.Fatalf("sensitive apply report=%+v error=%v", report, err)
	}
	if spy.opens != 1 {
		t.Fatalf("writer opens=%d, want 1", spy.opens)
	}
}

func TestPlanRefusesNonSensitiveColumnEvenWhenApproved(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"alpha", "beta", "gamma"})
	if !e.cfg.Approved {
		t.Fatal("fixture must exercise the approved path")
	}
	before := acceptanceTreeDigest(t, e.cfg.Target)
	plan, err := e.Plan(context.Background())
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("non-sensitive plan error=%v, want ErrUnsafe", err)
	}
	if plan.ID != "" || plan.ScanComplete {
		t.Fatalf("refused plan must be empty: %+v", plan)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err == nil {
		t.Fatalf("apply with an empty plan must fail, got report=%+v", report)
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("refused plan performed writes: %+v", spy)
	}
	if after := acceptanceTreeDigest(t, e.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("refused plan changed the target fixture tree")
	}
}

func TestPlanRefusesTruncatedSampleEvenWhenApproved(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid", "c@example.invalid"})
	e.cfg.SampleLimit = 2 // fewer than the distinct non-NULL values above
	if !e.cfg.Approved {
		t.Fatal("fixture must exercise the approved path")
	}
	before := acceptanceTreeDigest(t, e.cfg.Target)
	plan, err := e.Plan(context.Background())
	if !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("truncated sample plan error=%v, want ErrIncomplete", err)
	}
	if plan.ID != "" || plan.ScanComplete {
		t.Fatalf("refused plan must be empty: %+v", plan)
	}
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	if _, err := e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}); err == nil {
		t.Fatal("apply with an empty plan must fail")
	}
	if spy.opens != 0 {
		t.Fatalf("writer opened %d times for a refused plan", spy.opens)
	}
	if after := acceptanceTreeDigest(t, e.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("refused plan changed the target fixture tree")
	}
}

// TestScanReportAgreesWithPlanEvidence keeps Scan and Plan from drifting apart:
// both must derive completeness from the same sampling path.
func TestScanReportAgreesWithPlanEvidence(t *testing.T) {
	e, _ := acceptanceEngine(t, []string{"a@example.invalid", "b@example.invalid"})
	report, err := e.Scan(context.Background(), contracts.ScanRequest{})
	if err != nil {
		t.Fatalf("scan error=%v", err)
	}
	if !report.Coverage.Complete {
		t.Fatalf("sensitive scan coverage=%+v, want complete", report.Coverage)
	}
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan error=%v", err)
	}
	if plan.ScanComplete != report.Coverage.Complete {
		t.Fatalf("plan ScanComplete=%t disagrees with scan coverage=%t", plan.ScanComplete, report.Coverage.Complete)
	}
}

func TestApplyStillRequiresExplicitIntentAndApproval(t *testing.T) {
	for _, tc := range []struct {
		name     string
		explicit bool
		approved bool
	}{
		{"no explicit apply", false, true},
		{"no approval", true, false},
		{"neither", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := acceptanceEngine(t, []string{"a@example.invalid"})
			plan, err := e.Plan(context.Background())
			if err != nil {
				t.Fatalf("plan error=%v", err)
			}
			e.cfg.Approved = tc.approved
			spy := &writerSpy{}
			spyEngine(t, e, spy)
			if _, err = e.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: tc.explicit, BatchSize: 1}); !errors.Is(err, contracts.ErrUnsafe) {
				t.Fatalf("apply error=%v, want ErrUnsafe", err)
			}
			if spy.opens != 0 {
				t.Fatalf("writer opened %d times without authorization", spy.opens)
			}
		})
	}
}

// TestPlanRefusalNeverLeaksValues guards the sanitized reporting invariant on
// the new failure paths.
func TestPlanRefusalNeverLeaksValues(t *testing.T) {
	const marker = "SENSITIVE-SYNTHETIC-MARKER"
	e, _ := acceptanceEngine(t, []string{marker, marker + "-2"})
	e.cfg.SampleLimit = 1
	_, err := e.Plan(context.Background())
	if err == nil {
		t.Fatal("expected refusal")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("refusal error leaked a column value: %v", err)
	}
}
