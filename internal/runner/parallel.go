package runner

import (
	"context"
	"sync"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// maskBatch transforms an already bounded SQL page using a fixed worker pool.
// It neither reads nor writes a database. Results preserve input indexes even
// if workers finish out of order. Memory is O(page size + worker count).
func (e *Engine) maskBatch(ctx context.Context, original []contracts.Value, col contracts.ColumnPlan, workers int) ([]contracts.Value, error) {
	return maskBatchValues(ctx, original, col, workers, e.maskColumn)
}

// maskBatchValues has a private transform seam so cancellation, in-flight
// failures and goroutine cleanup can be tested deterministically.
func maskBatchValues(ctx context.Context, original []contracts.Value, col contracts.ColumnPlan, workers int, transform func(context.Context, contracts.Value, contracts.ColumnPlan) (contracts.Value, error)) ([]contracts.Value, error) {
	if ctx == nil || transform == nil || workers < 1 || workers > 16 || len(original) > 1000 {
		return nil, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]contracts.Value, len(original))
	if workers == 1 || len(original) < 2 {
		for i, v := range original {
			masked, err := transform(ctx, v, col)
			if err != nil {
				return nil, err
			}
			result[i] = masked
		}
		return result, nil
	}
	if workers > len(original) {
		workers = len(original)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int, workers)
	errors := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(workers)
	for n := 0; n < workers; n++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-workCtx.Done():
					return
				case i, open := <-jobs:
					if !open {
						return
					}
					value, err := transform(workCtx, original[i], col)
					if err != nil {
						select {
						case errors <- err:
						default:
						}
						cancel()
						return
					}
					result[i] = value
				}
			}
		}()
	}
	sent := 0
dispatch:
	for i := range original {
		select {
		case <-workCtx.Done():
			break dispatch
		case jobs <- i:
			sent++
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case err := <-errors:
		return nil, err
	default:
	}
	if sent != len(original) {
		return nil, contracts.ErrIncomplete
	}
	return result, nil
}
