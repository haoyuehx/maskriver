package runner

// Scale evidence for the streaming path. It is opt-in through
// MASKRIVER_WP3_SCALE so the normal test suite stays fast; the published
// evidence records exactly which cells ran. Synthetic data only.

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

type scaleCell struct {
	rows, workers, batch int
}

func scaleMode() string { return os.Getenv("MASKRIVER_WP3_SCALE") }

func TestStreamingScaleEvidence(t *testing.T) {
	mode := scaleMode()
	if mode == "" {
		t.Skip("set MASKRIVER_WP3_SCALE=quick|full to run streaming scale evidence")
	}
	outDir := os.Getenv("MASKRIVER_WP3_SCALE_DIR")
	if outDir == "" {
		t.Fatal("MASKRIVER_WP3_SCALE_DIR is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cells := []scaleCell{
		{128, 1, 64}, {512, 2, 64}, {2048, 4, 64}, {10000, 16, 64},
		{10000, 1, 64}, {10000, 2, 64}, {10000, 4, 64}, {10000, 8, 64}, {10000, 16, 64},
		{10000, 4, 16}, {10000, 4, 256},
		{100000, 8, 64},
	}
	if mode == "full" {
		cells = append(cells, scaleCell{100000, 1, 64}, scaleCell{100000, 16, 64})
	}
	file, err := os.Create(filepath.Join(outDir, "streaming-scale.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{
		"rows", "workers", "batch_size", "repeat", "apply_ms", "rows_per_s",
		"batches", "heap_alloc_bytes", "heap_sys_bytes", "total_alloc_bytes",
		"num_gc", "peak_goroutines", "peak_rss_kb",
	}); err != nil {
		t.Fatal(err)
	}
	repeats := 3
	if os.Getenv("MASKRIVER_WP3_SCALE_ONE_MILLION") == "1" {
		cells = []scaleCell{{1000000, 4, 256}}
		repeats = 1
	}
	for _, cell := range cells {
		for repeat := 0; repeat < repeats; repeat++ {
			row := runScaleCell(t, cell, repeat)
			if err := writer.Write(row); err != nil {
				t.Fatal(err)
			}
			writer.Flush()
		}
	}
}

func runScaleCell(t *testing.T, cell scaleCell, repeat int) []string {
	t.Helper()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	seedScaleDB(t, source, cell.rows)
	seedScaleDB(t, target, cell.rows)
	key, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	cfg := config.Runtime{
		Source: source, Target: target, Table: "people", Column: "email",
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 64, MinRatio: .9,
		BatchSize: cell.batch, Workers: cell.workers,
	}
	targetOpts, err := cfg.DatabaseOptions(false)
	if err != nil {
		t.Fatal(err)
	}
	targetOpts.MaxOpenConns = 2
	outboxOpts := db.OutboxOptions{
		DatabaseOptions: targetOpts, Protocol: contracts.StreamingProtocolRevision,
		JournalMode: "delete", MaxReceiptPage: testBudget.MaxReceiptPage,
	}
	if err := db.InstallOutboxSchema(context.Background(), outboxOpts); err != nil {
		t.Fatal(err)
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	plan, err := engine.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan rows=%d workers=%d: %v", cell.rows, cell.workers, err)
	}
	approval, record := streamingApproval(t, plan)
	engine.SetStreamingApprovalQuery(&fakeStreamingVerifier{record: record})
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	summary, err := engine.StreamingApply(context.Background(), contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: fmt.Sprintf("%032x", cell.rows*100+repeat),
		BatchSize: cell.batch, ReceiptPageSize: testBudget.MaxReceiptPage, Budget: testBudget,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("apply rows=%d workers=%d: %v", cell.rows, cell.workers, err)
	}
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if summary.State != contracts.RunStateComplete {
		t.Fatalf("rows=%d workers=%d state=%v", cell.rows, cell.workers, summary.State)
	}
	seconds := elapsed.Seconds()
	rate := float64(cell.rows) / seconds
	return []string{
		strconv.Itoa(cell.rows), strconv.Itoa(cell.workers), strconv.Itoa(cell.batch),
		strconv.Itoa(repeat), strconv.FormatInt(elapsed.Milliseconds(), 10),
		fmt.Sprintf("%.1f", rate), strconv.FormatInt(summary.BatchesCommitted, 10),
		strconv.FormatUint(after.HeapAlloc, 10), strconv.FormatUint(after.HeapSys, 10),
		strconv.FormatUint(after.TotalAlloc-before.TotalAlloc, 10),
		strconv.FormatUint(uint64(after.NumGC-before.NumGC), 10),
		strconv.Itoa(runtime.NumGoroutine()), strconv.FormatInt(peakRSSKB(), 10),
	}
}

// peakRSSKB reads the process high-water RSS on Linux. It is process-wide and
// cumulative, so it bounds a run rather than isolating it.
func peakRSSKB() int64 {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				value, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return value
				}
			}
		}
	}
	return -1
}

// seedScaleDB builds one synthetic table. Values are short, long, nullable and
// moderately repetitive; only 64 distinct values exist so sampling is complete
// without reading the whole table.
func seedScaleDB(t *testing.T, path string, rows int) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=journal_mode(delete)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, note TEXT NULL)`); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO people (id, email, note) VALUES (?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 50) + "@example.invalid"
	for i := 0; i < rows; i++ {
		var email any
		switch i % 8 {
		case 0:
			email = nil
		case 1:
			email = long
		default:
			email = fmt.Sprintf("user%02d@example.invalid", i%64)
		}
		if _, err := statement.Exec(i+1, email, "note"); err != nil {
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}
