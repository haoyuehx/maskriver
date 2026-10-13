package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestMaskBatchInFlightCancellation verifies that every worker exits before
// the transform returns, even when the dispatcher has already queued jobs.
func TestMaskBatchInFlightCancellation(t *testing.T) {
	_, col, values := syntheticBatch(t, 128)
	var active atomic.Int64
	started := make(chan struct{}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	transform := func(ctx context.Context, _ contracts.Value, _ contracts.ColumnPlan) (contracts.Value, error) {
		active.Add(1)
		defer active.Add(-1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return contracts.Value{}, ctx.Err()
	}
	_, err := maskBatchValues(ctx, values, col, 8, transform)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight cancellation did not propagate: %v", err)
	}
	if n := active.Load(); n != 0 {
		t.Fatalf("%d masking workers remained active", n)
	}
}

func TestMaskBatchInjectedFailureDrainsWorkers(t *testing.T) {
	_, col, values := syntheticBatch(t, 128)
	injected := errors.New("synthetic injected transform error")
	var active atomic.Int64
	var seen atomic.Bool
	transform := func(ctx context.Context, v contracts.Value, _ contracts.ColumnPlan) (contracts.Value, error) {
		active.Add(1)
		defer active.Add(-1)
		if seen.CompareAndSwap(false, true) {
			return contracts.Value{}, injected
		}
		<-ctx.Done()
		return contracts.Value{}, ctx.Err()
	}
	for i := 0; i < 40; i++ {
		seen.Store(false)
		_, err := maskBatchValues(context.Background(), values, col, 8, transform)
		if !errors.Is(err, injected) {
			t.Fatalf("root error was replaced by another worker's cancellation: %v", err)
		}
		if n := active.Load(); n != 0 {
			t.Fatalf("%d workers leaked after failure", n)
		}
	}
}

func TestMaskBatchRejectsUnboundedInputAndNilTransform(t *testing.T) {
	_, col, values := syntheticBatch(t, 1001)
	if _, err := maskBatchValues(context.Background(), values, col, 8, nil); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("nil transform accepted: %v", err)
	}
	if _, err := maskBatchValues(context.Background(), values, col, 8, func(_ context.Context, v contracts.Value, _ contracts.ColumnPlan) (contracts.Value, error) {
		return v, nil
	}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("unbounded input accepted: %v", err)
	}
}
