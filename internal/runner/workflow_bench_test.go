package runner

// Real SQLite workflow benchmarks for Issue #13.
//
// These drive the actual runner path — Scan, Plan, Preview, a real Ed25519
// signed decision stored in a real history store, Apply and the post-write
// validation — with every gate enabled. The worker pool, the approval check,
// preflight and the dry run are the production ones; nothing is stubbed to make
// the numbers look better.
//
// Scope note: this is a **Runner workflow** benchmark, not a full CLI E2E
// benchmark. The CLI path additionally parses flags and loads the key file; those
// costs are not in these numbers.
//
// Run:
//   go test -run '^$' -bench 'BenchmarkWorkflow' -benchmem -benchtime=3x ./internal/runner/

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// benchConfig is one row of the benchmark matrix.
type benchConfig struct {
	rows    int
	long    bool
	workers int
}

// benchMatrix is the phase-1 matrix: three sizes, two value shapes, two worker
// counts. Twelve configurations, each repeated by -benchtime.
func benchMatrix() []benchConfig {
	var out []benchConfig
	for _, rows := range []int{128, 512, 2048} {
		for _, long := range []bool{false, true} {
			for _, workers := range []int{1, 4} {
				out = append(out, benchConfig{rows: rows, long: long, workers: workers})
			}
		}
	}
	return out
}

// benchEmails builds synthetic values. Short is a plain address; long pads the
// local part so the value shape (not just the row count) varies.
func benchEmails(n int, long bool) []string {
	out := make([]string, n)
	for i := range out {
		if long {
			out[i] = fmt.Sprintf("synthetic-%08d-%s@example.invalid", i, strings.Repeat("x", 24))
			continue
		}
		out[i] = fmt.Sprintf("synthetic-%08d@example.invalid", i)
	}
	return out
}

// benchWorkflow runs one full workflow and returns per-stage durations. The
// fixture databases are built per iteration, so fixture cost stays outside the
// measurement and no state can leak between iterations.
// benchEnv is one prepared workflow environment: fixture databases, an open
// engine and a real history store. Building it is fixture cost and must never be
// inside a timed region.
type benchEnv struct {
	engine *Engine
	store  *history.Store
	base   config.Runtime
}

// close releases the engine's real reader and writer connections. Every measured
// iteration gets its own environment, so the target file is never overwritten
// while a connection still holds it open.
func (e *benchEnv) close() { _ = e.engine.Close() }

func benchEnvFor(tb testing.TB, cfg benchConfig) *benchEnv {
	tb.Helper()
	values := benchEmails(cfg.rows, cfg.long)
	base, key := benchFixture(tb, values)
	base.Workers = cfg.workers
	base.BatchSize = 64
	engine, err := Open(context.Background(), base, key)
	if err != nil {
		tb.Fatalf("Open: %v", err)
	}
	store, _ := benchStore(tb)
	engine.SetApprovalQuery(store)
	return &benchEnv{engine: engine, store: store, base: base}
}

// run executes one workflow on a prepared environment and returns the stage
// durations and the number of masked rows.
func (e *benchEnv) run(tb testing.TB) (stages map[string]time.Duration, rowsMasked int64) {
	tb.Helper()
	base, engine, store := e.base, e.engine, e.store
	ctx := context.Background()
	stages = make(map[string]time.Duration, 5)
	step := func(name string, fn func() error) {
		at := time.Now()
		if err := fn(); err != nil {
			tb.Fatalf("%s: %v", name, err)
		}
		stages[name] = time.Since(at)
	}

	var plan contracts.Plan
	step("scan", func() error {
		// Empty request: the engine derives the table set from the configured scope,
		// exactly as the CLI does.
		_, err := engine.Scan(ctx, contracts.ScanRequest{
			SampleLimit: base.SampleLimit, MinSamples: base.MinSamples, MinRatio: base.MinRatio,
		})
		return err
	})
	step("plan", func() error {
		p, err := engine.Plan(ctx)
		plan = p
		return err
	})
	step("preview", func() error {
		_, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
		return err
	})
	step("approve", func() error {
		_, err := store.Approve(ctx, history.ApproveRequest{
			Plan: plan, ReviewerID: testReviewerLabel,
			IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		})
		return err
	})
	var applied contracts.ApplyReport
	step("apply", func() error {
		rep, err := engine.Apply(ctx, contracts.ApplyRequest{
			Plan: plan, ExplicitApply: true, BatchSize: 64,
		})
		applied = rep
		return err
	})
	for _, r := range applied.Committed {
		rowsMasked += r.Rows
	}
	return stages, rowsMasked
}

// benchFixture builds two independent synthetic databases holding the same rows
// and returns a runtime config pointing at them. It mirrors acceptanceFixture but
// takes testing.TB so benchmarks can use it.
func benchFixture(tb testing.TB, rows []string) (config.Runtime, contracts.Value) {
	tb.Helper()
	dir := tb.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			tb.Fatal(err)
		}
		if _, err = database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`); err != nil {
			tb.Fatal(err)
		}
		for i, email := range rows {
			if _, err = database.Exec(`INSERT INTO people VALUES (?, ?)`, i+1, email); err != nil {
				tb.Fatal(err)
			}
		}
		if err = database.Close(); err != nil {
			tb.Fatal(err)
		}
	}
	key, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	return config.Runtime{
		Source: source, Target: target, Table: "people", Column: "email",
		Strategy:   contracts.StrategyRef{ID: "fake_email", Version: "1"},
		KeyID:      "synthetic",
		Approved:   true,
		MinSamples: 1, SampleLimit: 8192, MinRatio: .9, BatchSize: 64, Workers: 1,
	}, key
}

// benchStore builds a real history store with a freshly generated reviewer key so
// the benchmark exercises real signing and verification.
func benchStore(tb testing.TB) (*history.Store, ed25519.PrivateKey) {
	tb.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	trust, err := history.NewTrustedKeys(history.TrustedKey{SignerKeyID: testReviewerKeyID, Public: pub})
	if err != nil {
		tb.Fatal(err)
	}
	store, err := history.Open(history.Config{
		Dir:    filepath.Join(tb.TempDir(), "history"),
		Trust:  trust,
		Signer: &history.Signer{SignerKeyID: testReviewerKeyID, Private: priv},
	})
	if err != nil {
		tb.Fatal(err)
	}
	return store, priv
}

// BenchmarkWorkflow measures the whole runner workflow across the phase-1
// matrix. Reported metrics are the workflow total, per-stage medians and a
// throughput figure whose interval is the whole workflow (not the masking step).
func BenchmarkWorkflow(b *testing.B) {
	for _, cfg := range benchMatrix() {
		name := fmt.Sprintf("rows=%d/shape=%s/workers=%d", cfg.rows, shapeName(cfg.long), cfg.workers)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			// Fixture construction, connection teardown and target recreation all
			// happen with the timer stopped, and every iteration gets a fresh
			// environment so no state can carry over from the previous iteration.
			var rowsMasked int64
			stageSamples := map[string][]float64{}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				env := benchEnvFor(b, cfg)
				b.StartTimer()
				stages, rows := env.run(b)
				b.StopTimer()
				env.close()
				b.StartTimer()
				rowsMasked = rows
				for name, d := range stages {
					stageSamples[name] = append(stageSamples[name], float64(d.Microseconds()))
				}
			}
			b.StopTimer()
			if rowsMasked > 0 {
				// Interval: the timed workflow only, not fixture creation.
				b.ReportMetric(float64(rowsMasked)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
			}
			// Stage metrics are medians over the same iterations that make up ns/op,
			// not the last sample, so the numbers are on one footing.
			for name, samples := range stageSamples {
				b.ReportMetric(median(samples), "us_"+name+"_median")
			}
		})
	}
}

func shapeName(long bool) string {
	if long {
		return "long"
	}
	return "short"
}

// median returns the middle value of a small sample set. Reporting stage medians
// keeps the stage metrics on the same iterations as ns/op.
func median(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	if len(ordered)%2 == 1 {
		return ordered[len(ordered)/2]
	}
	return (ordered[len(ordered)/2-1] + ordered[len(ordered)/2]) / 2
}
