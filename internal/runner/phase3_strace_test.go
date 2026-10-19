package runner

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestPhase3StraceApply provides a clean, separately markered Apply-only
// interval. Run under Linux strace -f -ttt -T -yy -e trace=fsync,fdatasync,write.
// Fixture construction, Scan/Plan/Preview/signing happen before START; Validate
// and teardown happen after END. No statement text or bound data is printed.
func TestPhase3StraceApply(t *testing.T) {
	if os.Getenv("MASK_RIVER_STRACE_APPLY") == "" {
		t.Skip("set MASK_RIVER_STRACE_APPLY=1 and run under strace")
	}
	root := t.TempDir()
	env := benchEnvIn(t, benchConfig{rows: 256, workers: 4, batchSize: 64}, root)
	defer func() {
		if err := env.close(); err != nil {
			t.Errorf("close engine/remove fixture: %v", err)
		}
	}()
	ctx := context.Background()
	if _, err := env.engine.Scan(ctx, contracts.ScanRequest{}); err != nil {
		t.Fatal(err)
	}
	plan, err := env.engine.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 10000}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Approve(ctx, history.ApproveRequest{Plan: plan, ReviewerID: testReviewerLabel, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	spy := db.NewSQLSpy()
	attachDiagnosticWriter(env.engine, spy)
	fmt.Fprintf(os.Stderr, "PHASE3_APPLY_START unix_ns=%d target=%s\n", time.Now().UnixNano(), env.base.Target)
	report, err := env.engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 64})
	fmt.Fprintf(os.Stderr, "PHASE3_APPLY_END unix_ns=%d complete=%t receipts=%d\n", time.Now().UnixNano(), report.Complete, len(report.Committed))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || len(report.Committed) != 4 {
		t.Fatalf("unexpected apply report complete=%t receipts=%d", report.Complete, len(report.Committed))
	}
}
