package runner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func syntheticBatch(t testing.TB, n int) (*Engine, contracts.ColumnPlan, []contracts.Value) {
	t.Helper()
	key, err := contracts.NewValue(contracts.Bytes, "synthetic-worker-benchmark-key-material-40b")
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{key: key}
	col := contracts.ColumnPlan{
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"},
		Type:     contracts.Type{Kind: contracts.Text, Length: 128},
		Scope:    "synthetic/bench", NormalizationVersion: "v1", KeyID: "bench-only",
	}
	values := make([]contracts.Value, n)
	for i := range values {
		value, err := contracts.NewValue(contracts.Text, fmt.Sprintf("sample-%06d@example.invalid", i))
		if err != nil {
			t.Fatal(err)
		}
		values[i] = value
	}
	return engine, col, values
}

func TestMaskBatchDeterminismAndBoundedWorkers(t *testing.T) {
	engine, col, values := syntheticBatch(t, 128)
	reference, err := engine.maskBatch(context.Background(), values, col, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{2, 4, 8, 16} {
		for run := 0; run < 12; run++ {
			got, err := engine.maskBatch(context.Background(), values, col, workers)
			if err != nil || !reflect.DeepEqual(got, reference) {
				t.Fatal("parallel output differs from serial reference", err)
			}
		}
	}
	for _, n := range []int{0, 17} {
		if _, err := engine.maskBatch(context.Background(), values, col, n); !errors.Is(err, contracts.ErrInvalid) {
			t.Fatal("invalid worker count accepted", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.maskBatch(ctx, values, col, 8); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled mask operation accepted", err)
	}
	shortKey, _ := contracts.NewValue(contracts.Bytes, "too-short")
	bad := &Engine{key: shortKey}
	if _, err := bad.maskBatch(context.Background(), values, col, 8); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("worker error did not propagate", err)
	}
}

func BenchmarkMaskBatchWorkers(b *testing.B) {
	engine, col, values := syntheticBatch(b, 128)
	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				masked, err := engine.maskBatch(context.Background(), values, col, workers)
				if err != nil || len(masked) != len(values) {
					b.Fatal("synthetic batch transform failed", err)
				}
			}
		})
	}
}
