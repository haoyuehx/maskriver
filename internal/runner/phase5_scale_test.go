package runner

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	phase5BatchSize = 64 // captured production default; never changed by this test
	phase5SampleEnv = "MASKRIVER_PHASE5_SCALE_OUT"
	phase5ChildEnv  = "MASKRIVER_PHASE5_CHILD"
)

type phase5Case struct {
	Rows       int `json:"rows"`
	Workers    int `json:"workers"`
	Repetition int `json:"repetition"`
}

type phase5Sample struct {
	TrialID                    string           `json:"trial_id"`
	Rows                       int              `json:"rows"`
	Workers                    int              `json:"workers"`
	BatchSize                  int              `json:"batch_size"`
	Repetition                 int              `json:"repetition"`
	FixtureNS                  int64            `json:"fixture_ns"`
	FixtureSourceBytes         int64            `json:"fixture_source_bytes"`
	FixtureTargetBytes         int64            `json:"fixture_target_bytes"`
	WorkflowNS                 int64            `json:"workflow_ns"`
	ScanNS                     int64            `json:"scan_ns"`
	PlanNS                     int64            `json:"plan_ns"`
	PreviewNS                  int64            `json:"preview_ns"`
	ApproveNS                  int64            `json:"approve_ns"`
	ApplyNS                    int64            `json:"apply_ns"`
	ValidateNS                 int64            `json:"validate_ns"`
	VerificationNS             int64            `json:"verification_ns"`
	CleanupNS                  int64            `json:"cleanup_ns"`
	ProcessWallNS              int64            `json:"runner_test_process_wall_ns"`
	RowsPerSecond              float64          `json:"rows_per_second"`
	ApplyRowsPerSecond         float64          `json:"apply_rows_per_second"`
	Complete                   bool             `json:"complete"`
	ValidatePassed             bool             `json:"validate_passed"`
	RowsCommitted              int64            `json:"rows_committed"`
	ReceiptCount               int              `json:"receipt_count"`
	ReceiptCapacity            int              `json:"receipt_capacity"`
	ReceiptStructBytes         uintptr          `json:"receipt_struct_bytes"`
	ReceiptSliceCapacityBytes  uintptr          `json:"receipt_slice_capacity_bytes"`
	SuccessfulCommitCount      int              `json:"successful_commit_count"`
	CommitAttemptCount         int              `json:"commit_attempt_count"`
	BatchUpdateCallCount       int              `json:"batch_update_call_count"`
	TargetLogicalSHA256        string           `json:"target_logical_sha256"`
	SourceFileSHA256Before     string           `json:"source_file_sha256_before"`
	SourceFileSHA256After      string           `json:"source_file_sha256_after"`
	HighCardinalityDistinct    int64            `json:"high_cardinality_distinct"`
	HighCardinalityNulls       int64            `json:"high_cardinality_nulls"`
	RepeatedFieldDistinct      int64            `json:"repeated_field_distinct"`
	RepeatedFieldNulls         int64            `json:"repeated_field_nulls"`
	RepeatedFieldDuplicateRate float64          `json:"repeated_field_duplicate_rate"`
	TargetNullCountsMatch      bool             `json:"target_null_counts_match"`
	ChangedNonNullCounts       map[string]int64 `json:"changed_non_null_counts"`
	GoroutinesBefore           int              `json:"goroutines_before"`
	GoroutinesAfterClose       int              `json:"goroutines_after_close"`
	CPUUserNS                  int64            `json:"cpu_user_ns"`
	CPUSystemNS                int64            `json:"cpu_system_ns"`
	ReaderPoolOpenMax          int              `json:"reader_pool_open_max"`
	TargetPoolOpenMax          int              `json:"target_pool_open_max"`
	WriterPoolOpenMax          int              `json:"writer_pool_open_max"`
	WriterPoolInUseMax         int              `json:"writer_pool_in_use_max"`
	WriterPoolOpenAfterClose   int              `json:"writer_pool_open_after_close"`
	ResourceSampleCount        int              `json:"resource_sample_count"`
	PeakRSSKB                  int64            `json:"peak_rss_kb_sampled"`
	PeakHeapAllocBytes         uint64           `json:"peak_heap_alloc_bytes_sampled"`
	PeakHeapInuseBytes         uint64           `json:"peak_heap_inuse_bytes_sampled"`
	PeakGoroutines             int              `json:"peak_goroutines_sampled"`
	PeakFDs                    int              `json:"peak_fds_sampled"`
	PeakGCPauseTotalNS         uint64           `json:"peak_gc_pause_total_ns_sampled"`
	ResourceMetricsSupported   bool             `json:"resource_metrics_supported"`
	Resources                  []phase5Resource `json:"-"`
}

type phase5Resource struct {
	ElapsedMS      int64  `json:"elapsed_ms"`
	RSSKB          int64  `json:"rss_kb"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapInuseBytes uint64 `json:"heap_inuse_bytes"`
	NumGC          uint32 `json:"num_gc"`
	PauseTotalNS   uint64 `json:"pause_total_ns"`
	Goroutines     int    `json:"goroutines"`
	ReaderOpen     int    `json:"reader_open_connections"`
	ReaderInUse    int    `json:"reader_in_use_connections"`
	TargetOpen     int    `json:"target_open_connections"`
	TargetInUse    int    `json:"target_in_use_connections"`
	WriterOpen     int    `json:"writer_open_connections"`
	WriterInUse    int    `json:"writer_in_use_connections"`
	FDCount        int    `json:"fd_count"`
	CPUUserNS      int64  `json:"cpu_user_ns_process_cumulative"`
	CPUSystemNS    int64  `json:"cpu_system_ns_process_cumulative"`
}

// TestPhase5ScaleMatrix is opt-in. It launches one isolated test process per
// sample so peak RSS/CPU/heap do not inherit another row-count's high-water
// mark. Fixtures use one prepared transaction and are outside the measured
// Scan→Plan→Preview→Approve→Apply→Validate interval.
func TestPhase5ScaleMatrix(t *testing.T) {
	outDir := os.Getenv(phase5SampleEnv)
	if outDir == "" {
		t.Skip("set MASKRIVER_PHASE5_SCALE_OUT to run the 10k/100k/1M real-SQLite phase-5 matrix")
	}
	if runtime.GOOS != "linux" {
		t.Skip("continuous RSS/FD/CPU sampler is Linux-specific; use Linux for the phase-5 scale matrix")
	}
	outDir, err := filepath.Abs(outDir)
	if err != nil {
		t.Fatal("invalid phase-5 output directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal("create phase-5 output directory")
	}
	if _, err := os.Stat(filepath.Join(outDir, "samples.ndjson")); err == nil {
		t.Fatal("refusing to overwrite existing phase-5 raw samples")
	}
	budget := phase5ResourceBudget()
	writePhase5JSON(t, filepath.Join(outDir, "preflight.json"), budget)
	if !budget.Passed {
		t.Skip("phase-5 large-run budget insufficient; recorded in preflight.json")
	}

	matrix := phase5Matrix()
	writePhase5JSON(t, filepath.Join(outDir, "matrix.json"), map[string]any{
		"schema_version": 1, "batch_size": phase5BatchSize,
		"cases":               matrix,
		"field_distributions": phase5DistributionDefinitions(),
	})
	writePhase5Environment(t, outDir, budget)
	logPath := filepath.Join(outDir, "run-log.txt")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal("create measurement log")
	}
	defer func() { _ = log.Close() }()
	writer := bufio.NewWriter(log)

	completed := make([]phase5Sample, 0, len(matrix))
	for index, testCase := range matrix {
		trialID := fmt.Sprintf("n%d-w%d-r%d", testCase.Rows, testCase.Workers, testCase.Repetition)
		childDir := filepath.Join(outDir, "raw", trialID)
		if err := os.MkdirAll(childDir, 0o755); err != nil {
			t.Fatalf("create child output for %s", trialID)
		}
		configJSON, _ := json.Marshal(map[string]any{
			"case": testCase, "trial_id": trialID, "out_dir": childDir,
		})
		cmd := exec.Command(os.Args[0], "-test.run=^TestPhase5ScaleWorker$", "-test.count=1", "-test.timeout=1h")
		childEnv := withoutEnv(os.Environ(), phase5SampleEnv)
		childEnv = withoutEnv(childEnv, phase5ChildEnv)
		cmd.Env = append(childEnv, phase5ChildEnv+"="+string(configJSON))
		started := time.Now()
		output, runErr := cmd.CombinedOutput()
		processWall := time.Since(started).Nanoseconds()
		fmt.Fprintf(writer, "index=%d/%d trial=%s rows=%d workers=%d batch_size=%d process_wall_ns=%d exit=%t\n",
			index+1, len(matrix), trialID, testCase.Rows, testCase.Workers, phase5BatchSize, processWall, runErr == nil)
		_ = writer.Flush()
		if runErr != nil {
			sanitized := sanitizePhase5Output(string(output), outDir)
			_ = os.WriteFile(filepath.Join(outDir, "failed-child-output.txt"), []byte(sanitized), 0o600)
			t.Fatalf("phase-5 trial %s failed (exit status recorded; sanitized diagnostic: %s)", trialID, oneLine(sanitized))
		}
		var sample phase5Sample
		raw, err := os.ReadFile(filepath.Join(childDir, "sample.json"))
		if err != nil || json.Unmarshal(raw, &sample) != nil {
			t.Fatalf("phase-5 trial %s did not produce a readable raw sample", trialID)
		}
		sample.ProcessWallNS = processWall
		writePhase5JSON(t, filepath.Join(childDir, "sample.json"), sample)
		resources, err := readPhase5Resources(filepath.Join(childDir, "resource-samples.csv"))
		if err != nil {
			t.Fatalf("phase-5 trial %s resource samples invalid", trialID)
		}
		sample.Resources = resources
		if err := validatePhase5Sample(sample, testCase); err != nil {
			t.Fatalf("phase-5 trial %s failed sample validation: %v", trialID, err)
		}
		completed = append(completed, sample)
		appendPhase5JSONLine(t, filepath.Join(outDir, "samples.ndjson"), sample)
		writePhase5JSON(t, filepath.Join(outDir, "samples.json"), completed)
		writePhase5ResourceCSV(t, filepath.Join(outDir, "resource-samples.csv"), completed)
		if err := validatePhase5CrossWorker(completed); err != nil {
			t.Fatalf("phase-5 output mismatch: %v", err)
		}
	}
	if err := validatePhase5Matrix(completed, matrix); err != nil {
		t.Fatal(err)
	}
	writePhase5JSON(t, filepath.Join(outDir, "matrix.json"), map[string]any{
		"schema_version": 1, "batch_size": phase5BatchSize,
		"cases": matrix, "completed": len(completed),
		"field_distributions": phase5DistributionDefinitions(),
	})
}

// TestPhase5ScaleWorker runs one matrix cell in its own process. Its output is
// machine-readable, excludes cell values/keys, and separates fixture, workflow,
// independent result checking and teardown intervals.
func TestPhase5ScaleWorker(t *testing.T) {
	raw := os.Getenv(phase5ChildEnv)
	if raw == "" {
		t.Skip("phase-5 worker subprocess only")
	}
	var request struct {
		Case    phase5Case `json:"case"`
		TrialID string     `json:"trial_id"`
		OutDir  string     `json:"out_dir"`
	}
	if err := json.Unmarshal([]byte(raw), &request); err != nil || request.Case.Rows < 1 || request.Case.Workers < 1 || request.Case.Workers > 16 || request.OutDir == "" {
		t.Fatal("invalid phase-5 worker configuration")
	}
	t.Setenv(phase5SampleEnv, "")
	root := t.TempDir()
	fixtureStart := time.Now()
	cfg, key, sourceBytes, targetBytes, distribution, err := phase5Fixture(t, root, request.Case.Rows, request.Case.Workers)
	fixtureNS := time.Since(fixtureStart).Nanoseconds()
	if err != nil {
		t.Fatal("build synthetic SQLite fixture")
	}
	sourceBefore, err := phase5FileSHA(cfg.Source)
	if err != nil {
		t.Fatal("read synthetic source digest")
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatal("open real SQLite runner")
	}
	store, _, err := phase5ApprovalStore(filepath.Join(root, "history"))
	if err != nil {
		_ = engine.Close()
		t.Fatal("create synthetic Ed25519 approval store")
	}
	engine.SetApprovalQuery(store)
	writerObserver := &phase5WriterObserver{}
	openWriter := engine.openWriter
	engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		actual, openErr := openWriter(ctx, opts)
		if openErr != nil {
			return nil, openErr
		}
		writerObserver.setActual(actual)
		return &phase5ObservedWriter{Writer: actual, observer: writerObserver}, nil
	}

	goroutinesBefore := runtime.NumGoroutine()
	sampler := newPhase5Sampler(engine, writerObserver)
	workflowStarted := time.Now()
	beforeCPUUser, beforeCPUSys, cpuBeforeOK := cpuNS()
	stage := func(fn func() error) int64 {
		started := time.Now()
		if err := fn(); err != nil {
			t.Fatalf("phase-5 workflow stage failed (%s)", phase5SafeErr(err))
		}
		return time.Since(started).Nanoseconds()
	}
	var plan contracts.Plan
	scanNS := stage(func() error {
		_, callErr := engine.Scan(context.Background(), contracts.ScanRequest{})
		return callErr
	})
	planNS := stage(func() error {
		var callErr error
		plan, callErr = engine.Plan(context.Background())
		return callErr
	})
	previewNS := stage(func() error {
		_, callErr := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan})
		return callErr
	})
	approveNS := stage(func() error {
		_, callErr := store.Approve(context.Background(), history.ApproveRequest{
			Plan: plan, ReviewerID: "phase5-synthetic-reviewer",
			IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		})
		return callErr
	})
	var applied contracts.ApplyReport
	applyNS := stage(func() error {
		var callErr error
		applied, callErr = engine.Apply(context.Background(), contracts.ApplyRequest{
			Plan: plan, ExplicitApply: true, BatchSize: phase5BatchSize,
		})
		return callErr
	})
	var validation contracts.ValidationReport
	validateNS := stage(func() error {
		var callErr error
		validation, callErr = engine.Validate(context.Background(), contracts.ValidateRequest{Plan: plan})
		return callErr
	})
	workflowNS := time.Since(workflowStarted).Nanoseconds()
	afterCPUUser, afterCPUSys, cpuAfterOK := cpuNS()
	verificationStarted := time.Now()
	if !applied.Complete || len(applied.Committed) != (request.Case.Rows+phase5BatchSize-1)/phase5BatchSize || !validation.Coverage.Complete || !validation.Passed(true) || validation.Coverage.Rows != int64(request.Case.Rows) {
		t.Fatalf("phase-5 receipt or Validate invariants failed (complete=%t receipts=%d rows=%d coverage_complete=%t passed=%t)", applied.Complete, len(applied.Committed), validation.Coverage.Rows, validation.Coverage.Complete, validation.Passed(true))
	}
	var committed int64
	for i, receipt := range applied.Committed {
		if receipt.Ordinal != int64(i+1) || receipt.Table != tableRef("people") || receipt.Rows < 1 || receipt.Rows > phase5BatchSize {
			t.Fatal("invalid committed batch receipt")
		}
		committed += receipt.Rows
	}
	if committed != int64(request.Case.Rows) {
		t.Fatal("receipt rows do not match requested rows")
	}
	targetSHA, err := phase5LogicalDigest(cfg.Target)
	if err != nil {
		t.Fatal("check logical target data")
	}
	checks, err := phase5CheckSQL(cfg.Source, cfg.Target)
	if err != nil {
		t.Fatal("check source/target rows")
	}
	sourceAfter, err := phase5FileSHA(cfg.Source)
	if err != nil || sourceAfter != sourceBefore {
		t.Fatal("source database changed")
	}
	if !checks.TargetNullCountsMatch || checks.SourceHighDistinct != distribution.HighCardDistinct || checks.SourceHighNulls != distribution.HighCardNulls || checks.SourceRepeatedDistinct != distribution.RepeatedDistinct || checks.SourceRepeatedNulls != distribution.RepeatedNulls || checks.ChangedNonNullCounts["email_high_cardinality"] != int64(request.Case.Rows) || checks.ChangedNonNullCounts["email_repeated_null"] != int64(distribution.RepeatedNonNullRows) {
		t.Fatalf("row checks mismatch: null/distinct match=%t high-cardinality changed=%d/%d repeated non-NULL changed=%d/%d", checks.TargetNullCountsMatch, checks.ChangedNonNullCounts["email_high_cardinality"], request.Case.Rows, checks.ChangedNonNullCounts["email_repeated_null"], distribution.RepeatedNonNullRows)
	}
	verificationNS := time.Since(verificationStarted).Nanoseconds()
	resources := sampler.stop()
	_, _, poolOK := engine.PoolStats()
	if !poolOK {
		t.Fatal("database pool statistics unavailable")
	}
	writerOpenAfterClose := -1
	cleanupStart := time.Now()
	if err := engine.Close(); err != nil {
		t.Fatal("close runner resources")
	}
	if writerObserver.actual != nil {
		if stats, ok := db.DBStats(writerObserver.actual); ok {
			writerOpenAfterClose = stats.OpenConnections
		}
	}
	if readerAfter, targetAfter, ok := engine.PoolStats(); !ok || readerAfter.OpenConnections != 0 || targetAfter.OpenConnections != 0 {
		t.Fatal("reader connection pool not closed")
	}
	if writerOpenAfterClose != 0 {
		t.Fatal("writer connection pool not closed")
	}
	if err := phase5NoSidecars(root); err != nil {
		t.Fatal("SQLite sidecar remained after resource close")
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal("remove synthetic fixture and approval data")
	}
	cleanupNS := time.Since(cleanupStart).Nanoseconds()
	goroutinesAfter := waitPhase5Goroutines(goroutinesBefore, 2*time.Second)
	if goroutinesAfter > goroutinesBefore+2 {
		t.Fatalf("goroutines did not return near baseline after close: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
	userNS, sysNS := int64(-1), int64(-1)
	if cpuBeforeOK && cpuAfterOK {
		userNS, sysNS = afterCPUUser-beforeCPUUser, afterCPUSys-beforeCPUSys
	}
	writerOpenMax, writerInUseMax := writerObserver.maxOpen, writerObserver.maxInUse
	peakRSS, peakAlloc, peakInuse, peakG, peakFD, peakPause := phase5Peaks(resources)
	sample := phase5Sample{
		TrialID: request.TrialID, Rows: request.Case.Rows, Workers: request.Case.Workers, BatchSize: phase5BatchSize,
		Repetition: request.Case.Repetition, FixtureNS: fixtureNS, FixtureSourceBytes: sourceBytes, FixtureTargetBytes: targetBytes,
		WorkflowNS: workflowNS, ScanNS: scanNS, PlanNS: planNS, PreviewNS: previewNS, ApproveNS: approveNS,
		ApplyNS: applyNS, ValidateNS: validateNS, VerificationNS: verificationNS, CleanupNS: cleanupNS,
		RowsPerSecond:      float64(request.Case.Rows) / (float64(workflowNS) / 1e9),
		ApplyRowsPerSecond: float64(request.Case.Rows) / (float64(applyNS) / 1e9),
		Complete:           applied.Complete, ValidatePassed: validation.Coverage.Complete && validation.Passed(true),
		RowsCommitted: committed, ReceiptCount: len(applied.Committed), ReceiptCapacity: cap(applied.Committed),
		ReceiptStructBytes: unsafe.Sizeof(contracts.BatchReceipt{}), ReceiptSliceCapacityBytes: uintptr(cap(applied.Committed)) * unsafe.Sizeof(contracts.BatchReceipt{}),
		SuccessfulCommitCount: writerObserver.commitSuccess, CommitAttemptCount: writerObserver.commitAttempts,
		BatchUpdateCallCount: writerObserver.updateCalls, TargetLogicalSHA256: targetSHA,
		SourceFileSHA256Before: sourceBefore, SourceFileSHA256After: sourceAfter,
		HighCardinalityDistinct: distribution.HighCardDistinct, HighCardinalityNulls: distribution.HighCardNulls,
		RepeatedFieldDistinct: distribution.RepeatedDistinct, RepeatedFieldNulls: distribution.RepeatedNulls,
		RepeatedFieldDuplicateRate: distribution.RepeatedDuplicateRate, TargetNullCountsMatch: checks.TargetNullCountsMatch,
		ChangedNonNullCounts: checks.ChangedNonNullCounts, GoroutinesBefore: goroutinesBefore, GoroutinesAfterClose: goroutinesAfter,
		CPUUserNS: userNS, CPUSystemNS: sysNS, ReaderPoolOpenMax: phase5PeakPool(resources, "reader_open"),
		TargetPoolOpenMax: phase5PeakPool(resources, "target_open"), WriterPoolOpenMax: writerOpenMax, WriterPoolInUseMax: writerInUseMax,
		WriterPoolOpenAfterClose: writerOpenAfterClose, ResourceSampleCount: len(resources), PeakRSSKB: peakRSS,
		PeakHeapAllocBytes: peakAlloc, PeakHeapInuseBytes: peakInuse, PeakGoroutines: peakG, PeakFDs: peakFD,
		PeakGCPauseTotalNS: peakPause, ResourceMetricsSupported: peakRSS >= 0 && cpuBeforeOK && cpuAfterOK,
		Resources: resources,
	}
	writePhase5JSON(t, filepath.Join(request.OutDir, "sample.json"), sample)
	writePhase5ResourceCSV(t, filepath.Join(request.OutDir, "resource-samples.csv"), []phase5Sample{sample})
}

type phase5Distribution struct {
	HighCardDistinct      int64   `json:"email_high_cardinality_distinct"`
	HighCardNulls         int64   `json:"email_high_cardinality_nulls"`
	HighCardValueLength   int     `json:"email_high_cardinality_value_length_bytes"`
	HighCardDuplicateRate float64 `json:"email_high_cardinality_duplicate_rate_non_null"`
	RepeatedDistinct      int64   `json:"email_repeated_null_distinct_non_null"`
	RepeatedNulls         int64   `json:"email_repeated_null_null_rows"`
	RepeatedValueLength   int     `json:"email_repeated_null_value_length_bytes"`
	RepeatedDuplicateRate float64 `json:"email_repeated_null_duplicate_rate_non_null"`
	HighCardNonNullRows   int64   `json:"email_high_cardinality_non_null_rows"`
	RepeatedNonNullRows   int64   `json:"email_repeated_null_non_null_rows"`
}

func phase5Matrix() []phase5Case {
	var out []phase5Case
	for _, spec := range []struct{ rows, reps int }{{10_000, 5}, {100_000, 3}} {
		for worker := 1; worker <= 16; worker *= 2 {
			for rep := 1; rep <= spec.reps; rep++ {
				out = append(out, phase5Case{Rows: spec.rows, Workers: worker, Repetition: rep})
			}
		}
	}
	for _, worker := range []int{1, 4} {
		out = append(out, phase5Case{Rows: 1_000_000, Workers: worker, Repetition: 1})
	}
	return out
}

func phase5DistributionDefinitions() map[string]any {
	return map[string]any{
		"email_high_cardinality": map[string]any{
			"generator": "h%07d@example.invalid, index modulo 9500", "null_ratio": 0,
			"value_length_bytes": 24, "distinct_cap": 9500,
			"detection": "real runner DISTINCT non-NULL sample; cap 10000, minimum 20, ratio 0.90",
		},
		"email_repeated_null": map[string]any{
			"generator":          "NULL when zero-based row index mod 20 is 0; otherwise r%03d@example.invalid, index modulo 100",
			"null_ratio_formula": "exactly 5% for target row counts divisible by 20",
			"non_null_distinct":  95, "value_length_bytes": 20, "detection": "real runner DISTINCT non-NULL sample; cap 10000, minimum 20, ratio 0.90",
		},
		"notes": "Both fields are present in every run and use only synthetic reserved .invalid values. duplicate rate depends on row count and is calculated from actual fixture aggregates.",
	}
}

func phase5Fixture(t *testing.T, root string, rows, workers int) (config.Runtime, contracts.Value, int64, int64, phase5Distribution, error) {
	t.Helper()
	source, target := filepath.Join(root, "source.db"), filepath.Join(root, "target.db")
	database, err := sql.Open("sqlite", "file:"+source+"?mode=rwc")
	if err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	database.SetMaxOpenConns(1)
	if _, err = database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email_high_cardinality TEXT NOT NULL, email_repeated_null TEXT NULL)`); err != nil {
		_ = database.Close()
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	tx, err := database.Begin()
	if err != nil {
		_ = database.Close()
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	stmt, err := tx.Prepare(`INSERT INTO people(id,email_high_cardinality,email_repeated_null) VALUES(?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		_ = database.Close()
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	nulls, repeatedDistinct, highDistinct := int64(0), int64(0), int64(rows)
	highPool := rows
	if highPool > 9500 {
		highPool = 9500
	}
	if highPool > 0 {
		highDistinct = int64(highPool)
	}
	if rows >= 100 {
		repeatedDistinct = 95
	} else {
		repeatedDistinct = int64((rows+19)/20) - 1
		if repeatedDistinct < 0 {
			repeatedDistinct = 0
		}
	}
	for i := 0; i < rows; i++ {
		high := fmt.Sprintf("h%07d@example.invalid", i%9500)
		var repeated any
		if i%20 == 0 {
			repeated = nil
			nulls++
		} else {
			repeated = fmt.Sprintf("r%03d@example.invalid", i%100)
		}
		if _, err = stmt.Exec(i+1, high, repeated); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			_ = database.Close()
			return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
		}
	}
	if err = stmt.Close(); err != nil {
		_ = tx.Rollback()
		_ = database.Close()
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	if err = tx.Commit(); err != nil {
		_ = database.Close()
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	if err = database.Close(); err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	if err = phase5CopyFile(source, target); err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("P", 32))
	if err != nil {
		return config.Runtime{}, contracts.Value{}, 0, 0, phase5Distribution{}, err
	}
	cfg := config.Runtime{
		Source: source, Target: target,
		Scopes:   []config.Scope{{Table: "people", Column: "email_high_cardinality"}, {Table: "people", Column: "email_repeated_null"}},
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "phase5-synthetic-key",
		Approved: true, MinSamples: 20, SampleLimit: 10000, MinRatio: .9, BatchSize: phase5BatchSize, Workers: workers,
	}
	highDup := 1.0 - float64(highDistinct)/float64(rows)
	nonNull := int64(rows) - nulls
	repeatedDup := 0.0
	if nonNull > 0 {
		repeatedDup = 1 - float64(repeatedDistinct)/float64(nonNull)
	}
	distribution := phase5Distribution{HighCardDistinct: highDistinct, HighCardNulls: 0, HighCardValueLength: len("h0000000@example.invalid"), HighCardDuplicateRate: highDup,
		RepeatedDistinct: repeatedDistinct, RepeatedNulls: nulls, RepeatedValueLength: len("r000@example.invalid"), RepeatedDuplicateRate: repeatedDup,
		HighCardNonNullRows: int64(rows), RepeatedNonNullRows: nonNull}
	return cfg, key, sourceInfo.Size(), targetInfo.Size(), distribution, nil
}

func phase5ApprovalStore(dir string) (*history.Store, any, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	trust, err := history.NewTrustedKeys(history.TrustedKey{SignerKeyID: "phase5-reviewer", Public: pub})
	if err != nil {
		return nil, nil, err
	}
	store, err := history.Open(history.Config{Dir: dir, Trust: trust, Signer: &history.Signer{SignerKeyID: "phase5-reviewer", Private: priv}})
	return store, priv, err
}

func phase5CopyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	return errors.Join(copyErr, closeErr)
}

func phase5FileSHA(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func phase5LogicalDigest(path string) (string, error) {
	file, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer file.Close()
	rows, err := file.Query(`SELECT id,email_high_cardinality,email_repeated_null FROM people ORDER BY id`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	var id int64
	var high string
	var repeated sql.NullString
	count := 0
	for rows.Next() {
		if err := rows.Scan(&id, &high, &repeated); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%d:%d:%s:%t:%s\n", id, len(high), high, repeated.Valid, repeated.String)
		count++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", fmt.Errorf("empty target")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type phase5SQLChecks struct {
	TargetNullCountsMatch  bool
	SourceHighDistinct     int64
	SourceHighNulls        int64
	SourceRepeatedDistinct int64
	SourceRepeatedNulls    int64
	ChangedNonNullCounts   map[string]int64
}

func phase5CheckSQL(source, target string) (phase5SQLChecks, error) {
	combined, err := sql.Open("sqlite", "file:"+source+"?mode=ro")
	if err != nil {
		return phase5SQLChecks{}, err
	}
	defer combined.Close()
	combined.SetMaxOpenConns(1)
	if _, err = combined.Exec(`ATTACH DATABASE ? AS target`, "file:"+target+"?mode=ro"); err != nil {
		return phase5SQLChecks{}, err
	}
	var rows, highSame, repeatSame, srcHighNull, dstHighNull, srcNull, dstNull int64
	var srcHighDistinct, dstHighDistinct, srcRepeatDistinct, dstRepeatDistinct int64
	query := `SELECT COUNT(*), SUM(s.email_high_cardinality=t.email_high_cardinality), SUM(s.email_repeated_null IS t.email_repeated_null), SUM(s.email_high_cardinality IS NULL), SUM(t.email_high_cardinality IS NULL), SUM(s.email_repeated_null IS NULL), SUM(t.email_repeated_null IS NULL), COUNT(DISTINCT s.email_high_cardinality), COUNT(DISTINCT t.email_high_cardinality), COUNT(DISTINCT s.email_repeated_null), COUNT(DISTINCT t.email_repeated_null) FROM main.people AS s JOIN target.people AS t USING(id)`
	if err = combined.QueryRow(query).Scan(&rows, &highSame, &repeatSame, &srcHighNull, &dstHighNull, &srcNull, &dstNull, &srcHighDistinct, &dstHighDistinct, &srcRepeatDistinct, &dstRepeatDistinct); err != nil {
		return phase5SQLChecks{}, err
	}
	checks := phase5SQLChecks{
		TargetNullCountsMatch: srcHighNull == dstHighNull && srcNull == dstNull && srcHighDistinct == dstHighDistinct && srcRepeatDistinct == dstRepeatDistinct,
		SourceHighDistinct:    srcHighDistinct, SourceHighNulls: srcHighNull,
		SourceRepeatedDistinct: srcRepeatDistinct, SourceRepeatedNulls: srcNull,
		ChangedNonNullCounts: map[string]int64{
			"email_high_cardinality": rows - highSame,
			"email_repeated_null":    rows - repeatSame,
		},
	}
	return checks, nil
}

func validatePhase5Sample(s phase5Sample, c phase5Case) error {
	if s.Rows != c.Rows || s.Workers != c.Workers || s.Repetition != c.Repetition || s.BatchSize != phase5BatchSize || s.WorkflowNS <= 0 || s.ApplyNS <= 0 || s.ValidateNS <= 0 || s.FixtureNS <= 0 {
		return errors.New("configuration or duration mismatch")
	}
	stageSum := s.ScanNS + s.PlanNS + s.PreviewNS + s.ApproveNS + s.ApplyNS + s.ValidateNS
	if stageSum > s.WorkflowNS || s.RowsCommitted != int64(c.Rows) || !s.Complete || !s.ValidatePassed || s.ReceiptCount != (c.Rows+phase5BatchSize-1)/phase5BatchSize || s.SuccessfulCommitCount != s.ReceiptCount || s.CommitAttemptCount != s.SuccessfulCommitCount || s.BatchUpdateCallCount != s.ReceiptCount {
		return errors.New("workflow/receipt/commit invariant mismatch")
	}
	if s.SourceFileSHA256Before == "" || s.SourceFileSHA256Before != s.SourceFileSHA256After || !s.TargetNullCountsMatch || s.ResourceSampleCount < 2 || s.PeakRSSKB < 0 || s.PeakHeapAllocBytes == 0 || s.TargetLogicalSHA256 == "" {
		return errors.New("source, target, or resource evidence missing")
	}
	if s.ChangedNonNullCounts["email_high_cardinality"] != int64(c.Rows) || s.ChangedNonNullCounts["email_repeated_null"] < 0 {
		return errors.New("row-level transformed-value checks failed")
	}
	return nil
}

func validatePhase5CrossWorker(samples []phase5Sample) error {
	hashes := map[string]string{}
	for _, s := range samples {
		key := fmt.Sprintf("%d/%d", s.Rows, s.Repetition)
		if prior, ok := hashes[key]; ok && prior != s.TargetLogicalSHA256 {
			return fmt.Errorf("worker-dependent durable output for %s", key)
		}
		hashes[key] = s.TargetLogicalSHA256
	}
	return nil
}
func validatePhase5Matrix(samples []phase5Sample, expected []phase5Case) error {
	if len(samples) != len(expected) {
		return fmt.Errorf("completed %d/%d matrix cells", len(samples), len(expected))
	}
	seen := map[string]bool{}
	for _, sample := range samples {
		key := fmt.Sprintf("%d/%d/%d", sample.Rows, sample.Workers, sample.Repetition)
		if seen[key] {
			return fmt.Errorf("duplicate matrix cell %s", key)
		}
		seen[key] = true
	}
	for _, cell := range expected {
		key := fmt.Sprintf("%d/%d/%d", cell.Rows, cell.Workers, cell.Repetition)
		if !seen[key] {
			return fmt.Errorf("missing matrix cell %s", key)
		}
	}
	return nil
}

func writePhase5Environment(t *testing.T, out string, budget phase5Budget) {
	t.Helper()
	data := map[string]any{"measurement_code_sha": currentGitSHA(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "go_version": runtime.Version(), "cpu_count": runtime.NumCPU(), "host_cpu": hostCPU(), "kernel": hostKernel(), "host_memory_total_mb": hostMemoryTotalMB(), "memory_source": "/proc/meminfo host value; container limit checked separately if exposed", "sqlite_driver": "modernc.org/sqlite v1.60.1", "batch_size": phase5BatchSize, "journal_mode": "DELETE/default rollback journal", "synchronous": "driver default; not changed by test", "repetitions": map[string]int{"10000": 5, "100000": 3, "1000000": 1}, "fixture": "source built in one prepared transaction; target copied after close; construction excluded from workflow", "workflow": "Scan, Plan, Preview, Ed25519 Approve, Apply, independent Validate", "resource_sample_period_ms": 100, "resource_budget": budget, "command": "MASKRIVER_PHASE5_SCALE_OUT=<dir> go test -run '^TestPhase5ScaleMatrix$' -count=1 -timeout 4h ./internal/runner"}
	writePhase5JSON(t, filepath.Join(out, "environment.json"), data)
}

type phase5Budget struct {
	AvailableDiskBytes           uint64 `json:"available_disk_bytes"`
	AvailableMemoryKB            int64  `json:"available_memory_kb"`
	EstimatedPeakWorkingSetBytes uint64 `json:"conservative_estimated_peak_working_set_bytes"`
	MinimumDiskBytes             uint64 `json:"minimum_disk_bytes"`
	MinimumMemoryKB              int64  `json:"minimum_available_memory_kb"`
	Passed                       bool   `json:"passed"`
	Source                       string `json:"source"`
}

func phase5ResourceBudget() phase5Budget {
	availableDisk, diskSource := phase5DiskAvailable()
	budget := phase5Budget{AvailableDiskBytes: availableDisk, MinimumDiskBytes: 5 << 30, MinimumMemoryKB: 4 << 20, EstimatedPeakWorkingSetBytes: 2 << 30, Source: diskSource + " and /proc/meminfo"}
	data, _ := os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				budget.AvailableMemoryKB, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}
	budget.Passed = budget.AvailableDiskBytes >= budget.MinimumDiskBytes && budget.AvailableMemoryKB >= budget.MinimumMemoryKB
	return budget
}
func phase5DistributionForRows(rows int) phase5Distribution {
	high := rows
	if high > 9500 {
		high = 9500
	}
	nulls := int64((rows + 19) / 20)
	nonNull := int64(rows) - nulls
	distinct := int64(95)
	if rows < 100 {
		distinct = int64(rows) - nulls
		if distinct < 0 {
			distinct = 0
		}
	}
	highDup := 1 - float64(high)/float64(rows)
	repeatDup := 0.0
	if nonNull > 0 {
		repeatDup = 1 - float64(distinct)/float64(nonNull)
	}
	return phase5Distribution{HighCardDistinct: int64(high), HighCardValueLength: len("h0000000@example.invalid"), HighCardDuplicateRate: highDup, HighCardNonNullRows: int64(rows), RepeatedDistinct: distinct, RepeatedNulls: nulls, RepeatedValueLength: len("r000@example.invalid"), RepeatedDuplicateRate: repeatDup, RepeatedNonNullRows: nonNull}
}

func appendPhase5JSONLine(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal("append phase-5 sample")
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	if err := enc.Encode(value); err != nil {
		t.Fatal("encode phase-5 sample")
	}
}
func writePhase5JSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal("encode phase-5 artifact")
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal("write phase-5 artifact")
	}
}
func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, key+"=") {
			out = append(out, item)
		}
	}
	return out
}

var phase5SyntheticValue = regexp.MustCompile(`(?:h[0-9]{7}|r[0-9]{3})@example\.invalid|P{32}`)

func sanitizePhase5Output(text, root string) string {
	text = strings.ReplaceAll(text, root, "<phase5-output>")
	return phase5SyntheticValue.ReplaceAllString(text, "<synthetic-redacted>")
}
func oneLine(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 240 {
		return value[:240]
	}
	return value
}
func phase5SafeErr(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context deadline exceeded"
	case errors.Is(err, contracts.ErrUnsafe):
		return "safety gate refused"
	case errors.Is(err, contracts.ErrIncomplete):
		return "incomplete coverage"
	case errors.Is(err, contracts.ErrDatabase):
		return "database operation failed"
	default:
		return "workflow stage failed"
	}
}
func waitPhase5Goroutines(baseline int, wait time.Duration) int {
	deadline := time.Now().Add(wait)
	current := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		if current <= baseline+2 {
			return current
		}
		time.Sleep(10 * time.Millisecond)
		current = runtime.NumGoroutine()
	}
	return current
}
func phase5Peaks(samples []phase5Resource) (int64, uint64, uint64, int, int, uint64) {
	rss := int64(-1)
	alloc, inuse := uint64(0), uint64(0)
	goroutines, fds := 0, 0
	pause := uint64(0)
	for _, s := range samples {
		if s.RSSKB > rss {
			rss = s.RSSKB
		}
		if s.HeapAllocBytes > alloc {
			alloc = s.HeapAllocBytes
		}
		if s.HeapInuseBytes > inuse {
			inuse = s.HeapInuseBytes
		}
		if s.Goroutines > goroutines {
			goroutines = s.Goroutines
		}
		if s.FDCount > fds {
			fds = s.FDCount
		}
		if s.PauseTotalNS > pause {
			pause = s.PauseTotalNS
		}
	}
	return rss, alloc, inuse, goroutines, fds, pause
}
func phase5PeakPool(samples []phase5Resource, which string) int {
	peak := 0
	for _, s := range samples {
		v := s.ReaderOpen
		if which == "target_open" {
			v = s.TargetOpen
		}
		if v > peak {
			peak = v
		}
	}
	return peak
}

func newPhase5Sampler(engine *Engine, writer *phase5WriterObserver) *phase5Sampler {
	sampler := &phase5Sampler{engine: engine, writer: writer, start: time.Now(), stopCh: make(chan struct{}), done: make(chan struct{})}
	sampler.rows = append(sampler.rows, sampler.snapshot())
	go sampler.loop()
	return sampler
}

type phase5Sampler struct {
	engine *Engine
	writer *phase5WriterObserver
	start  time.Time
	stopCh chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	rows   []phase5Resource
}

func (s *phase5Sampler) loop() {
	defer close(s.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			s.record()
			return
		case <-ticker.C:
			s.record()
		}
	}
}
func (s *phase5Sampler) record() {
	row := s.snapshot()
	s.mu.Lock()
	s.rows = append(s.rows, row)
	s.mu.Unlock()
}
func (s *phase5Sampler) snapshot() phase5Resource {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	reader, target, ok := s.engine.PoolStats()
	r := phase5Resource{ElapsedMS: time.Since(s.start).Milliseconds(), RSSKB: rssKB(), HeapAllocBytes: mem.HeapAlloc, HeapInuseBytes: mem.HeapInuse, NumGC: mem.NumGC, PauseTotalNS: mem.PauseTotalNs, Goroutines: runtime.NumGoroutine(), ReaderOpen: -1, ReaderInUse: -1, TargetOpen: -1, TargetInUse: -1, WriterOpen: -1, WriterInUse: -1, FDCount: phase5FDCount()}
	if ok {
		r.ReaderOpen, r.ReaderInUse = reader.OpenConnections, reader.InUse
		r.TargetOpen, r.TargetInUse = target.OpenConnections, target.InUse
	}
	if user, sys, ok := cpuNS(); ok {
		r.CPUUserNS, r.CPUSystemNS = user, sys
	} else {
		r.CPUUserNS, r.CPUSystemNS = -1, -1
	}
	if s.writer != nil {
		open, inuse := s.writer.stats()
		r.WriterOpen, r.WriterInUse = open, inuse
	}
	return r
}
func (s *phase5Sampler) stop() []phase5Resource {
	close(s.stopCh)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]phase5Resource(nil), s.rows...)
}

func writePhase5ResourceCSV(t *testing.T, path string, samples []phase5Sample) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal("write resource samples")
	}
	defer file.Close()
	w := csv.NewWriter(file)
	_ = w.Write([]string{"trial_id", "elapsed_ms", "rss_kb", "heap_alloc_bytes", "heap_inuse_bytes", "num_gc", "pause_total_ns", "goroutines", "reader_open", "reader_in_use", "target_open", "target_in_use", "writer_open", "writer_in_use", "fd_count", "cpu_user_ns_process_cumulative", "cpu_system_ns_process_cumulative"})
	for _, sample := range samples {
		for _, r := range sample.Resources {
			_ = w.Write([]string{sample.TrialID, strconv.FormatInt(r.ElapsedMS, 10), strconv.FormatInt(r.RSSKB, 10), strconv.FormatUint(r.HeapAllocBytes, 10), strconv.FormatUint(r.HeapInuseBytes, 10), strconv.FormatUint(uint64(r.NumGC), 10), strconv.FormatUint(r.PauseTotalNS, 10), strconv.Itoa(r.Goroutines), strconv.Itoa(r.ReaderOpen), strconv.Itoa(r.ReaderInUse), strconv.Itoa(r.TargetOpen), strconv.Itoa(r.TargetInUse), strconv.Itoa(r.WriterOpen), strconv.Itoa(r.WriterInUse), strconv.Itoa(r.FDCount), strconv.FormatInt(r.CPUUserNS, 10), strconv.FormatInt(r.CPUSystemNS, 10)})
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal("flush resource samples")
	}
}
func readPhase5Resources(path string) ([]phase5Resource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil || len(header) < 17 {
		return nil, errors.New("invalid resource sample header")
	}
	var out []phase5Resource
	for {
		line, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		numbers := make([]int64, 17)
		for i := 1; i < 17; i++ {
			numbers[i], err = strconv.ParseInt(line[i], 10, 64)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, phase5Resource{ElapsedMS: numbers[1], RSSKB: numbers[2], HeapAllocBytes: uint64(numbers[3]), HeapInuseBytes: uint64(numbers[4]), NumGC: uint32(numbers[5]), PauseTotalNS: uint64(numbers[6]), Goroutines: int(numbers[7]), ReaderOpen: int(numbers[8]), ReaderInUse: int(numbers[9]), TargetOpen: int(numbers[10]), TargetInUse: int(numbers[11]), WriterOpen: int(numbers[12]), WriterInUse: int(numbers[13]), FDCount: int(numbers[14]), CPUUserNS: numbers[15], CPUSystemNS: numbers[16]})
	}
	return out, nil
}

type phase5WriterObserver struct {
	mu                                                                                                 sync.Mutex
	actual                                                                                             contracts.Writer
	maxOpen, maxInUse                                                                                  int
	beginAttempts, updateCalls, commitAttempts, commitSuccess, commitErrors, unknown, rollbackAttempts int
}

func (o *phase5WriterObserver) setActual(actual contracts.Writer) {
	o.mu.Lock()
	o.actual = actual
	o.mu.Unlock()
}

func (o *phase5WriterObserver) stats() (int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	stats, ok := db.DBStats(o.actual)
	if !ok {
		return -1, -1
	}
	if stats.OpenConnections > o.maxOpen {
		o.maxOpen = stats.OpenConnections
	}
	if stats.InUse > o.maxInUse {
		o.maxInUse = stats.InUse
	}
	return stats.OpenConnections, stats.InUse
}

type phase5ObservedWriter struct {
	contracts.Writer
	observer *phase5WriterObserver
}

func (w *phase5ObservedWriter) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	w.observer.mu.Lock()
	w.observer.beginAttempts++
	w.observer.mu.Unlock()
	tx, err := w.Writer.Begin(ctx, plan)
	w.observer.stats()
	if err != nil {
		return nil, err
	}
	return &phase5ObservedTx{WriteTx: tx, observer: w.observer}, nil
}
func (w *phase5ObservedWriter) Close() error { err := w.Writer.Close(); w.observer.stats(); return err }

type phase5ObservedTx struct {
	contracts.WriteTx
	observer *phase5WriterObserver
}

func (tx *phase5ObservedTx) Update(ctx context.Context, rows []contracts.RowChange) (int64, error) {
	tx.observer.mu.Lock()
	tx.observer.updateCalls++
	tx.observer.mu.Unlock()
	n, err := tx.WriteTx.Update(ctx, rows)
	tx.observer.stats()
	return n, err
}
func (tx *phase5ObservedTx) Commit(ctx context.Context) error {
	tx.observer.mu.Lock()
	tx.observer.commitAttempts++
	tx.observer.mu.Unlock()
	err := tx.WriteTx.Commit(ctx)
	tx.observer.mu.Lock()
	if err == nil {
		tx.observer.commitSuccess++
	} else {
		tx.observer.commitErrors++
		if errors.Is(err, contracts.ErrCommitUnknown) {
			tx.observer.unknown++
		}
	}
	tx.observer.mu.Unlock()
	tx.observer.stats()
	return err
}
func (tx *phase5ObservedTx) Rollback(ctx context.Context) error {
	tx.observer.mu.Lock()
	tx.observer.rollbackAttempts++
	tx.observer.mu.Unlock()
	err := tx.WriteTx.Rollback(ctx)
	tx.observer.stats()
	return err
}

func phase5NoSidecars(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") || strings.HasSuffix(name, "-journal") {
			return fmt.Errorf("SQLite sidecar %s remains", name)
		}
	}
	return nil
}

func phase5HostFileCount() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}
func phase5FDCount() int { return phase5HostFileCount() }
