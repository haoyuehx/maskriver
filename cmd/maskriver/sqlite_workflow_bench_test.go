package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/runner"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// BenchmarkSQLiteM1Workflow runs a REAL SQLite Reader -> Preview -> Writer ->
// independent snapshot validation workflow. Source files and keyed data are
// synthetic and disposable. Only runtime, not fixture setup/reset, is timed.
//
// Run with -benchtime=1x -count=3 for row-count/distribution comparisons.
// The benchmark includes SQL disk I/O and schema lookup, but NOT CLI parsing,
// external process startup, production workloads, or steady-state guarantees.
func BenchmarkSQLiteM1Workflow(b *testing.B) {
	for _, count := range []int{128, 512, 2048} {
		for _, shape := range []string{"short", "long"} {
			for _, workers := range []int{1, 4} {
				name := fmt.Sprintf("rows_%d/%s/workers_%d", count, shape, workers)
				b.Run(name, func(b *testing.B) {
					dir := b.TempDir()
					source := filepath.Join(dir, "source.db")
					target := filepath.Join(dir, "target.db")
					seedSQLiteBenchmark(b, source, count, shape)
					original, err := os.ReadFile(source)
					if err != nil {
						b.Fatal("read synthetic SQLite fixture:", err)
					}
					key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("K", 40))
					if err != nil {
						b.Fatal("synthetic key:", err)
					}
					cfg := config.Runtime{
						Source: source, Target: target,
						Table: "people", Column: "email", Approved: true,
						Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"},
						KeyID: "benchmark-key", MinSamples: 1, SampleLimit: 64,
						MinRatio: 1, BatchSize: 128, Workers: workers,
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						if err := os.WriteFile(target, original, 0600); err != nil {
							b.Fatal("reset target fixture:", err)
						}
						b.StartTimer()
						ctx := context.Background()
						engine, err := runner.Open(ctx, cfg, key)
						if err != nil {
							b.Fatal("reader open:", err)
						}
						plan, err := engine.Plan(ctx)
						if err == nil {
							_, err = engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
						}
						if err == nil {
							var report contracts.ApplyReport
							report, err = engine.Apply(ctx, contracts.ApplyRequest{
								Plan: plan, BatchSize: cfg.BatchSize, ExplicitApply: true,
							})
							if err == nil && (!report.Complete || len(report.Committed) == 0) {
								err = contracts.ErrIncomplete
							}
						}
						if err == nil {
							var report contracts.ValidationReport
							report, err = engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
							if err == nil && (!report.Passed(true) || report.Coverage.Rows != int64(count)) {
								err = contracts.ErrIncomplete
							}
						}
						closeErr := engine.Close()
						if err != nil || closeErr != nil {
							b.Fatal("SQLite workflow or resource close failed:", err, closeErr)
						}
						b.StopTimer()
					}
					if d := b.Elapsed().Seconds(); d > 0 {
						b.ReportMetric(float64(count*b.N)/d, "rows/s")
					}
				})
			}
		}
	}
}

func seedSQLiteBenchmark(b *testing.B, path string, count int, shape string) {
	b.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=foreign_keys(1)")
	if err != nil {
		b.Fatal("seed open:", err)
	}
	defer sqlDB.Close()
	if _, err := sqlDB.Exec("CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)"); err != nil {
		b.Fatal("seed schema:", err)
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		b.Fatal("seed begin:", err)
	}
	stmt, err := tx.Prepare("INSERT INTO people(id,email) VALUES(?,?)")
	if err != nil {
		_ = tx.Rollback()
		b.Fatal("seed statement:", err)
	}
	for i := 1; i <= count; i++ {
		local := fmt.Sprintf("synthetic-%07d", i)
		if shape == "long" {
			local = fmt.Sprintf("longsynthetic%07dtestaccount", i)
		}
		if _, err := stmt.Exec(i, local+"@example.invalid"); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			b.Fatal("seed insertion:", err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		b.Fatal("close insert statement:", err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal("seed commit:", err)
	}
}
