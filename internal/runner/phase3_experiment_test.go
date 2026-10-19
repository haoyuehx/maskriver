package runner

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// phase3Trial is one independently constructed, signed-approval, real SQLite
// workflow. Resource deltas cover Scan through Apply only; fixture time is
// separate and Validate is checked afterwards but excluded from workflow timing.
type phase3Trial struct {
	Order                 int     `json:"order"`
	Trial                 int     `json:"trial"`
	BatchSize             int     `json:"batch_size"`
	Rows                  int     `json:"rows"`
	Workers               int     `json:"workers"`
	FixtureNS             int64   `json:"fixture_ns"`
	WorkflowNS            int64   `json:"workflow_ns"`
	ApplyNS               int64   `json:"apply_ns"`
	RowsPerSecond         float64 `json:"rows_per_second"`
	BeginAttempts         int     `json:"begin_attempts"`
	BeginSucceeded        int     `json:"begin_succeeded"`
	BeginFailed           int     `json:"begin_failed"`
	CommitAttempts        int     `json:"commit_attempts"`
	CommitReturnedSuccess int     `json:"commit_returned_success"`
	CommitReturnedError   int     `json:"commit_returned_error"`
	CommitUnknown         int     `json:"commit_unknown"`
	RollbackAttempts      int     `json:"rollback_attempts"`
	RollbackSucceeded     int     `json:"rollback_succeeded"`
	RollbackFailed        int     `json:"rollback_failed"`
	BeginWallNS           int64   `json:"begin_wall_ns_including_writer_validation"`
	CommitWallNS          int64   `json:"commit_wall_ns_wrapper"`
	RollbackWallNS        int64   `json:"rollback_wall_ns_wrapper"`
	UpdateCallWallNS      int64   `json:"update_call_wall_ns_including_validation"`
	DriverBeginNS         int64   `json:"driver_begin_ns"`
	DriverCommitNS        int64   `json:"driver_commit_ns"`
	DriverRollbackNS      int64   `json:"driver_rollback_ns"`
	UpdateExecNS          int64   `json:"driver_update_exec_ns"`
	OtherSQLCallNS        int64   `json:"driver_other_sql_call_ns"`
	Statements            int     `json:"sql_statements"`
	UpdateStatements      int     `json:"update_statements"`
	BatchTransactionNS    []int64 `json:"batch_transaction_ns"`
	BatchTransactionSumNS int64   `json:"batch_transaction_sum_ns"`
	ApplyUnassignedNS     int64   `json:"apply_unassigned_ns"`
	ReceiptRows           []int64 `json:"receipt_rows"`
	JournalMode           string  `json:"journal_mode"`
	Synchronous           string  `json:"synchronous"`
	BatchStatementCounts  []int   `json:"statements_per_batch"`
	BatchUpdateCounts     []int   `json:"updates_per_batch"`
	BatchOtherCounts      []int   `json:"other_statements_per_batch"`
	BatchBeginDriverNS    []int64 `json:"driver_begin_ns_per_batch"`
	BatchCommitDriverNS   []int64 `json:"driver_commit_ns_per_batch"`
	BatchUpdateExecNS     []int64 `json:"driver_update_exec_ns_per_batch"`
	BatchOtherSQLCallNS   []int64 `json:"driver_other_sql_call_ns_per_batch"`
	Receipts              int     `json:"receipts"`
	RowsCommitted         int64   `json:"rows_committed"`
	Complete              bool    `json:"complete"`
	ValidatePassed        bool    `json:"independent_validate_passed"`
	GoroutinesBefore      int     `json:"goroutines_before"`
	GoroutinesAfter       int     `json:"goroutines_after"`
	RSSBeforeKB           int64   `json:"rss_before_kb"`
	RSSAfterKB            int64   `json:"rss_after_kb"`
	CPUUserNS             int64   `json:"cpu_user_ns"`
	CPUSystemNS           int64   `json:"cpu_system_ns"`
	ReaderOpen            int     `json:"reader_open_connections"`
	WriterOpen            int     `json:"writer_open_connections"`
	ReaderOpenAfterClose  int     `json:"reader_open_connections_after_close"`
	WriterOpenAfterClose  int     `json:"writer_open_connections_after_close"`
}

func attachDiagnosticWriter(engine *Engine, sqlSpy *db.SQLSpy) **spyBatchWriter {
	var observed *spyBatchWriter
	engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
		writer, err := sqlSpy.OpenWriter(ctx, opts)
		if err != nil {
			return nil, err
		}
		observed = &spyBatchWriter{Writer: writer, sqlSpy: sqlSpy}
		return observed, nil
	}
	return &observed
}

func runPhase3Trial(t *testing.T, root string, order, trial, batch, rows, workers int) phase3Trial {
	t.Helper()
	startedFixture := time.Now()
	env := benchEnvIn(t, benchConfig{rows: rows, workers: workers, batchSize: batch}, root)
	fixtureNS := time.Since(startedFixture).Nanoseconds()
	closed := false
	defer func() {
		if !closed {
			if err := env.close(); err != nil {
				t.Errorf("close engine/remove fixture: %v", err)
			}
		}
	}()

	sqlSpy := db.NewSQLSpy()
	observedPtr := attachDiagnosticWriter(env.engine, sqlSpy)
	beforeCPUUser, beforeCPUSys, cpuOK := cpuNS()
	beforeRSS, beforeG := rssKB(), runtime.NumGoroutine()
	started := time.Now()
	stages, rowsDone := env.run(t)
	workflowNS := time.Since(started).Nanoseconds()
	afterCPUUser, afterCPUSys, cpuAfterOK := cpuNS()
	afterRSS, afterG := rssKB(), runtime.NumGoroutine()
	reader, writer, poolOK := env.engine.PoolStats()

	validation, err := env.engine.Validate(context.Background(), contracts.ValidateRequest{Plan: env.lastPlan})
	if err != nil {
		t.Fatalf("independent Validate: %v", err)
	}
	validatePassed := validation.Coverage.Complete && validation.Passed(true)
	if !env.lastApply.Complete || rowsDone != int64(rows) || !validatePassed {
		t.Fatalf("batch=%d workers=%d apply complete=%v rows=%d validate=%v coverage=%+v", batch, workers, env.lastApply.Complete, rowsDone, validatePassed, validation.Coverage)
	}
	if int64(len(env.lastApply.Committed)) <= 0 {
		t.Fatal("Apply produced no receipts")
	}

	observed := *observedPtr
	if observed == nil {
		t.Fatal("writer instrumentation did not observe writer open")
	}
	observed.mu.Lock()
	bA, bS, bF := observed.beginAttempts, observed.beginSucceeded, observed.beginFailed
	cA, cS, cF, cU := observed.commitAttempts, observed.commitSucceeded, observed.commitFailed, observed.commitUnknown
	rA, rS, rF := observed.rollbackAttempts, observed.rollbackSucceeded, observed.rollbackFailed
	bWall, cWall, rWall, uWall := observed.beginTime, observed.commitTime, observed.rollbackTime, observed.updateTime
	batchWindows := append([]time.Duration(nil), observed.batchWindows...)
	batchStatementCounts := append([]int(nil), observed.batchStatementCounts...)
	batchUpdateCounts := append([]int(nil), observed.batchUpdateCounts...)
	batchOtherCounts := append([]int(nil), observed.batchOtherCounts...)
	batchBeginDriverNS := append([]int64(nil), observed.batchBeginDriverNS...)
	batchCommitDriverNS := append([]int64(nil), observed.batchCommitDriverNS...)
	batchUpdateExecNS := append([]int64(nil), observed.batchUpdateExecNS...)
	batchOtherSQLCallNS := append([]int64(nil), observed.batchOtherSQLCallNS...)
	observed.mu.Unlock()

	events := sqlSpy.Events()
	var driverBegin, driverCommit, driverRollback, updateExec, otherSQL int64
	for _, event := range events {
		switch event.Verb {
		case "BEGIN":
			driverBegin += event.Duration.Nanoseconds()
		case "COMMIT":
			driverCommit += event.Duration.Nanoseconds()
		case "ROLLBACK":
			driverRollback += event.Duration.Nanoseconds()
		case "UPDATE":
			updateExec += event.Duration.Nanoseconds()
		default:
			otherSQL += event.Duration.Nanoseconds()
		}
	}
	statements := sqlSpy.Statements()
	updates := 0
	for _, s := range statements {
		if s.Verb == "UPDATE" {
			updates++
		}
	}
	var committed int64
	var receiptRows []int64
	for _, receipt := range env.lastApply.Committed {
		committed += receipt.Rows
		receiptRows = append(receiptRows, receipt.Rows)
	}
	if rowsDone != committed {
		t.Fatalf("rowsMasked=%d differs from receipt rows=%d", rowsDone, committed)
	}
	if bA != len(env.lastApply.Committed) || bS != bA || bF != 0 || cA != len(env.lastApply.Committed) || cS != cA || cF != 0 || cU != 0 || rA != 0 || rF != 0 {
		t.Fatalf("unexpected transaction outcomes: begin %d/%d/%d commit %d/%d/%d unknown=%d rollback %d/%d/%d receipts=%d", bA, bS, bF, cA, cS, cF, cU, rA, rS, rF, len(env.lastApply.Committed))
	}
	applyNS := stages["apply"].Nanoseconds()
	if len(batchWindows) != len(env.lastApply.Committed) {
		t.Fatalf("instrumented transaction windows=%d receipts=%d", len(batchWindows), len(env.lastApply.Committed))
	}
	batchWindowNS := make([]int64, len(batchWindows))
	var batchWindowSum int64
	for i, elapsed := range batchWindows {
		batchWindowNS[i] = elapsed.Nanoseconds()
		batchWindowSum += batchWindowNS[i]
	}
	if batchWindowSum > applyNS {
		t.Fatalf("sum transaction windows %d exceeds Apply stage %d", batchWindowSum, applyNS)
	}
	if !poolOK {
		t.Fatal("Engine.PoolStats unavailable")
	}
	journal, synchronous := sqliteModes(env.base.Target)
	trialResult := phase3Trial{
		Order: order, Trial: trial, BatchSize: batch, Rows: rows, Workers: workers,
		JournalMode: journal, Synchronous: synchronous, ReceiptRows: receiptRows,
		FixtureNS: fixtureNS, WorkflowNS: workflowNS, ApplyNS: applyNS,
		RowsPerSecond: float64(rows) / (float64(applyNS) / 1e9),
		BeginAttempts: bA, BeginSucceeded: bS, BeginFailed: bF,
		CommitAttempts: cA, CommitReturnedSuccess: cS, CommitReturnedError: cF, CommitUnknown: cU,
		RollbackAttempts: rA, RollbackSucceeded: rS, RollbackFailed: rF,
		BeginWallNS: bWall.Nanoseconds(), CommitWallNS: cWall.Nanoseconds(), RollbackWallNS: rWall.Nanoseconds(),
		UpdateCallWallNS: uWall.Nanoseconds(), DriverBeginNS: driverBegin, DriverCommitNS: driverCommit,
		DriverRollbackNS: driverRollback, UpdateExecNS: updateExec, OtherSQLCallNS: otherSQL,
		Statements: len(statements), UpdateStatements: updates, BatchStatementCounts: batchStatementCounts,
		BatchUpdateCounts: batchUpdateCounts, BatchOtherCounts: batchOtherCounts,
		BatchBeginDriverNS: batchBeginDriverNS, BatchCommitDriverNS: batchCommitDriverNS,
		BatchUpdateExecNS: batchUpdateExecNS, BatchOtherSQLCallNS: batchOtherSQLCallNS,
		BatchTransactionNS:    batchWindowNS,
		BatchTransactionSumNS: batchWindowSum, ApplyUnassignedNS: applyNS - batchWindowSum,
		Receipts: len(env.lastApply.Committed), RowsCommitted: committed,
		Complete: env.lastApply.Complete, ValidatePassed: validatePassed,
		GoroutinesBefore: beforeG, GoroutinesAfter: afterG, RSSBeforeKB: beforeRSS, RSSAfterKB: afterRSS,
		CPUUserNS: -1, CPUSystemNS: -1, ReaderOpen: -1, WriterOpen: -1,
	}
	if cpuOK && cpuAfterOK {
		trialResult.CPUUserNS, trialResult.CPUSystemNS = afterCPUUser-beforeCPUUser, afterCPUSys-beforeCPUSys
	}
	if afterRSS < 0 || beforeRSS < 0 {
		trialResult.RSSBeforeKB, trialResult.RSSAfterKB = -1, -1
	}
	trialResult.ReaderOpen, trialResult.WriterOpen = reader.OpenConnections, writer.OpenConnections
	if err := env.close(); err != nil {
		t.Fatalf("close engine/remove fixture: %v", err)
	}
	closed = true
	readerClosed, writerClosed, closeStatsOK := env.engine.PoolStats()
	if !closeStatsOK || readerClosed.OpenConnections != 0 || writerClosed.OpenConnections != 0 {
		t.Fatalf("connection pools remain after close: ok=%v reader=%d writer=%d", closeStatsOK, readerClosed.OpenConnections, writerClosed.OpenConnections)
	}
	trialResult.ReaderOpenAfterClose, trialResult.WriterOpenAfterClose = readerClosed.OpenConnections, writerClosed.OpenConnections
	return trialResult
}

func TestPhase3BatchExperiment(t *testing.T) {
	outDir := os.Getenv("MASK_RIVER_PHASE3_DIR")
	if outDir == "" {
		t.Skip("set MASK_RIVER_PHASE3_DIR to run the 30-trial SQLite BatchSize experiment")
	}
	repetitions := 10
	if raw := os.Getenv("MASK_RIVER_PHASE3_REPS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			t.Fatalf("invalid MASK_RIVER_PHASE3_REPS=%q", raw)
		}
		repetitions = parsed
	}
	order := make([]int, 0, repetitions*4)
	for i := 0; i < repetitions; i++ {
		order = append(order, 16, 64, 128, 256)
	}
	rand.New(rand.NewSource(0x13_2026)).Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	root := t.TempDir()
	results := make([]phase3Trial, 0, len(order))
	seen := map[int]int{}
	for i, batch := range order {
		seen[batch]++
		results = append(results, runPhase3Trial(t, root, i+1, seen[batch], batch, 256, 4))
	}
	for _, batch := range []int{16, 64, 128, 256} {
		var commits, receipts, rows int64
		for _, sample := range results {
			if sample.BatchSize == batch {
				commits += int64(sample.CommitReturnedSuccess)
				receipts += int64(sample.Receipts)
				rows += sample.RowsCommitted
			}
		}
		if commits != receipts || rows != int64(repetitions*256) {
			t.Fatalf("batch=%d commit success=%d receipts=%d rows=%d", batch, commits, receipts, rows)
		}
	}
	writePhase3Artifacts(t, outDir, results, repetitions)
	t.Logf("wrote %d randomized trials across batch 16/64/128/256 to %s", len(results), outDir)
}

func writePhase3Artifacts(t *testing.T, dir string, trials []phase3Trial, repetitions int) {
	t.Helper()
	if err := validatePhase3Trials(trials, repetitions); err != nil {
		t.Fatalf("invalid phase-3 trial samples: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonBytes, err := json.MarshalIndent(trials, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "samples.json"), string(jsonBytes)+"\n")
	var rows strings.Builder
	w := csv.NewWriter(&rows)
	_ = w.Write([]string{"order", "trial", "batch_size", "rows", "workers", "fixture_ns", "workflow_ns", "apply_ns", "apply_rows_per_s", "begin_attempts", "begin_succeeded", "begin_failed", "commit_attempts", "commit_returned_success", "commit_returned_error", "commit_unknown", "rollback_attempts", "rollback_succeeded", "rollback_failed", "begin_wall_ns", "commit_wall_ns", "rollback_wall_ns", "update_call_wall_ns", "driver_begin_ns", "driver_commit_ns", "driver_rollback_ns", "driver_update_exec_ns", "driver_other_sql_call_ns", "statements", "update_statements", "receipts", "rows_committed", "batch_transaction_sum_ns", "apply_unassigned_ns", "complete", "validate_passed", "goroutines_before", "goroutines_after", "rss_before_kb", "rss_after_kb", "cpu_user_ns", "cpu_system_ns", "reader_open", "writer_open", "reader_open_after_close", "writer_open_after_close"})
	for _, s := range trials {
		_ = w.Write([]string{strconv.Itoa(s.Order), strconv.Itoa(s.Trial), strconv.Itoa(s.BatchSize), strconv.Itoa(s.Rows), strconv.Itoa(s.Workers), i64(s.FixtureNS), i64(s.WorkflowNS), i64(s.ApplyNS), f64(s.RowsPerSecond), strconv.Itoa(s.BeginAttempts), strconv.Itoa(s.BeginSucceeded), strconv.Itoa(s.BeginFailed), strconv.Itoa(s.CommitAttempts), strconv.Itoa(s.CommitReturnedSuccess), strconv.Itoa(s.CommitReturnedError), strconv.Itoa(s.CommitUnknown), strconv.Itoa(s.RollbackAttempts), strconv.Itoa(s.RollbackSucceeded), strconv.Itoa(s.RollbackFailed), i64(s.BeginWallNS), i64(s.CommitWallNS), i64(s.RollbackWallNS), i64(s.UpdateCallWallNS), i64(s.DriverBeginNS), i64(s.DriverCommitNS), i64(s.DriverRollbackNS), i64(s.UpdateExecNS), i64(s.OtherSQLCallNS), strconv.Itoa(s.Statements), strconv.Itoa(s.UpdateStatements), strconv.Itoa(s.Receipts), i64(s.RowsCommitted), i64(s.BatchTransactionSumNS), i64(s.ApplyUnassignedNS), strconv.FormatBool(s.Complete), strconv.FormatBool(s.ValidatePassed), strconv.Itoa(s.GoroutinesBefore), strconv.Itoa(s.GoroutinesAfter), i64(s.RSSBeforeKB), i64(s.RSSAfterKB), i64(s.CPUUserNS), i64(s.CPUSystemNS), strconv.Itoa(s.ReaderOpen), strconv.Itoa(s.WriterOpen), strconv.Itoa(s.ReaderOpenAfterClose), strconv.Itoa(s.WriterOpenAfterClose)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apply-transactions.csv"), rows.String())
	writeFile(t, filepath.Join(dir, "apply-batches.csv"), phase3BatchCSV(t, trials))
	writeFile(t, filepath.Join(dir, "results.json"), renderPhase3Stats(trials))
	writeFile(t, filepath.Join(dir, "stats.csv"), renderPhase3CSVStats(trials))
	writeFile(t, filepath.Join(dir, "results.csv"), renderPhase3ResultsCSV(trials))
	writeFile(t, filepath.Join(dir, "summary.md"), renderPhase3Summary(trials, repetitions))
	writeFile(t, filepath.Join(dir, "environment.json"), phase3Environment(trials, repetitions))
	writeFile(t, filepath.Join(dir, "resource-metrics.json"), phase3Resources(trials))
	writeFile(t, filepath.Join(dir, "throughput.svg"), phase3SVG(trials))
}

func phase3BatchCSV(t *testing.T, trials []phase3Trial) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("order,trial,batch_size,batch_index,statements,update_statements,other_statements,driver_begin_ns,driver_commit_ns,driver_update_exec_ns,driver_other_sql_call_ns,transaction_window_ns,receipt_rows,complete\n")
	for _, sample := range trials {
		if len(sample.BatchStatementCounts) != sample.Receipts || len(sample.BatchUpdateCounts) != sample.Receipts || len(sample.BatchOtherCounts) != sample.Receipts || len(sample.BatchBeginDriverNS) != sample.Receipts || len(sample.BatchCommitDriverNS) != sample.Receipts || len(sample.BatchUpdateExecNS) != sample.Receipts || len(sample.BatchOtherSQLCallNS) != sample.Receipts || len(sample.ReceiptRows) != sample.Receipts {
			t.Fatalf("trial %d batch detail mismatch: stmts=%d updates=%d receipts=%d rows=%d", sample.Order, len(sample.BatchStatementCounts), len(sample.BatchUpdateCounts), sample.Receipts, len(sample.ReceiptRows))
		}
		for i := range sample.BatchStatementCounts {
			fmt.Fprintf(&b, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%t\n", sample.Order, sample.Trial, sample.BatchSize, i+1, sample.BatchStatementCounts[i], sample.BatchUpdateCounts[i], sample.BatchOtherCounts[i], sample.BatchBeginDriverNS[i], sample.BatchCommitDriverNS[i], sample.BatchUpdateExecNS[i], sample.BatchOtherSQLCallNS[i], sample.BatchTransactionNS[i], sample.ReceiptRows[i], sample.Complete)
		}
	}
	return b.String()
}

func renderPhase3Stats(samples []phase3Trial) string {
	stats := map[string]map[string]float64{}
	for _, b := range []int{16, 64, 128, 256} {
		var applies, rowsPS, commits, begin, updates, rss, cpu []float64
		for _, s := range samples {
			if s.BatchSize == b {
				applies = append(applies, float64(s.ApplyNS))
				rowsPS = append(rowsPS, s.RowsPerSecond)
				commits = append(commits, float64(s.DriverCommitNS))
				begin = append(begin, float64(s.DriverBeginNS))
				updates = append(updates, float64(s.UpdateExecNS))
				if s.RSSAfterKB >= 0 {
					rss = append(rss, float64(s.RSSAfterKB))
				}
				if s.CPUUserNS >= 0 {
					cpu = append(cpu, float64(s.CPUUserNS))
				}
			}
		}
		stats[strconv.Itoa(b)] = map[string]float64{"apply_mean_ns": statsOf(applies).Mean, "apply_median_ns": statsOf(applies).Median, "apply_min_ns": statsOf(applies).Min, "apply_max_ns": statsOf(applies).Max, "apply_stddev_pct": statsOf(applies).StdDevPct, "rows_per_s_mean": statsOf(rowsPS).Mean, "rows_per_s_median": statsOf(rowsPS).Median, "commit_driver_mean_ns": statsOf(commits).Mean, "begin_driver_mean_ns": statsOf(begin).Mean, "update_exec_mean_ns": statsOf(updates).Mean, "rss_after_kb_median": statsOf(rss).Median, "cpu_user_ns_median": statsOf(cpu).Median}
	}
	b, _ := json.MarshalIndent(stats, "", "  ")
	return string(b) + "\n"
}

func renderPhase3ResultsCSV(samples []phase3Trial) string {
	var b strings.Builder
	b.WriteString("batch_size,iterations,commits_per_trial,rows_per_trial,apply_mean_ms,apply_median_ms,apply_min_ms,apply_max_ms,apply_stddev_pct,apply_rows_per_s_mean,apply_us_per_row,driver_commit_mean_ms,driver_update_exec_mean_ms,batch_transaction_window_mean_ms,apply_unassigned_mean_ms\n")
	for _, size := range []int{16, 64, 128, 256} {
		var applies, throughput, commits, driverCommit, updates, txWindows, residual []float64
		for _, s := range samples {
			if s.BatchSize == size {
				applies = append(applies, float64(s.ApplyNS))
				throughput = append(throughput, s.RowsPerSecond)
				commits = append(commits, float64(s.CommitReturnedSuccess))
				driverCommit = append(driverCommit, float64(s.DriverCommitNS))
				updates = append(updates, float64(s.UpdateExecNS))
				txWindows = append(txWindows, float64(s.BatchTransactionSumNS))
				residual = append(residual, float64(s.ApplyUnassignedNS))
			}
		}
		as := statsOf(applies)
		fmt.Fprintf(&b, "%d,%d,%.0f,256,%.3f,%.3f,%.3f,%.3f,%.2f,%.2f,%.2f,%.3f,%.3f,%.3f,%.3f\n", size, len(applies), statsOf(commits).Mean, as.Mean/1e6, as.Median/1e6, as.Min/1e6, as.Max/1e6, as.StdDevPct, statsOf(throughput).Mean, as.Mean/256/1e3, statsOf(driverCommit).Mean/1e6, statsOf(updates).Mean/1e6, statsOf(txWindows).Mean/1e6, statsOf(residual).Mean/1e6)
	}
	return b.String()
}

func renderPhase3CSVStats(samples []phase3Trial) string {
	var b strings.Builder
	b.WriteString("batch_size,metric,count,mean,median,min,max,stddev_pct\n")
	for _, size := range []int{16, 64, 128, 256} {
		metrics := map[string][]float64{}
		for _, s := range samples {
			if s.BatchSize != size {
				continue
			}
			metrics["fixture_ns"] = append(metrics["fixture_ns"], float64(s.FixtureNS))
			metrics["workflow_ns"] = append(metrics["workflow_ns"], float64(s.WorkflowNS))
			metrics["apply_ns"] = append(metrics["apply_ns"], float64(s.ApplyNS))
			metrics["apply_rows_per_s"] = append(metrics["apply_rows_per_s"], s.RowsPerSecond)
			metrics["driver_begin_ns"] = append(metrics["driver_begin_ns"], float64(s.DriverBeginNS))
			metrics["driver_commit_ns"] = append(metrics["driver_commit_ns"], float64(s.DriverCommitNS))
			metrics["update_exec_ns"] = append(metrics["update_exec_ns"], float64(s.UpdateExecNS))
			metrics["other_sql_call_ns"] = append(metrics["other_sql_call_ns"], float64(s.OtherSQLCallNS))
			metrics["batch_transaction_window_ns"] = append(metrics["batch_transaction_window_ns"], float64(s.BatchTransactionSumNS))
			metrics["apply_unassigned_ns"] = append(metrics["apply_unassigned_ns"], float64(s.ApplyUnassignedNS))
			if s.RSSAfterKB >= 0 {
				metrics["rss_after_kb"] = append(metrics["rss_after_kb"], float64(s.RSSAfterKB))
			}
			if s.CPUUserNS >= 0 {
				metrics["cpu_user_ns"] = append(metrics["cpu_user_ns"], float64(s.CPUUserNS))
				metrics["cpu_system_ns"] = append(metrics["cpu_system_ns"], float64(s.CPUSystemNS))
			}
		}
		names := make([]string, 0, len(metrics))
		for name := range metrics {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stats := statsOf(metrics[name])
			fmt.Fprintf(&b, "%d,%s,%d,%.1f,%.1f,%.1f,%.1f,%.2f\n", size, name, stats.Count, stats.Mean, stats.Median, stats.Min, stats.Max, stats.StdDevPct)
		}
	}
	return b.String()
}

func renderPhase3Summary(samples []phase3Trial, reps int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Issue #13 Phase 3 — transaction observation (%d randomized repeats/config)\n\n", reps)
	b.WriteString("All rows are synthetic `@example.invalid`; each trial uses a fresh source/target and real Ed25519 approval. Fixture build is outside workflow/apply intervals. Apply has a complete receipt and passed independent full Validate on every row. See `samples.json` and `apply-transactions.csv` for every independent trial.\n\n")
	b.WriteString("| Batch | commits/trial | Apply mean/median (ms) | min–max ms | CV % | Apply rows/s mean | Begin driver (ms) | Commit driver (ms) | UPDATE exec (ms) | Other SQL call (ms) | Transaction windows (ms) | Apply residual (ms) |\n|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, size := range []int{16, 64, 128, 256} {
		var a, r, begin, commit, update, other, transactionWindow, residual []float64
		commitCount := -1
		for _, s := range samples {
			if s.BatchSize == size {
				a = append(a, float64(s.ApplyNS))
				r = append(r, s.RowsPerSecond)
				begin = append(begin, float64(s.DriverBeginNS))
				commit = append(commit, float64(s.DriverCommitNS))
				update = append(update, float64(s.UpdateExecNS))
				other = append(other, float64(s.OtherSQLCallNS))
				transactionWindow = append(transactionWindow, float64(s.BatchTransactionSumNS))
				residual = append(residual, float64(s.ApplyUnassignedNS))
				commitCount = s.CommitAttempts
			}
		}
		as := statsOf(a)
		fmt.Fprintf(&b, "| %d | %d | %.2f / %.2f | %.2f–%.2f | %.1f | %.0f | %.3f | %.3f | %.3f | %.3f | %.2f | %.2f |\n", size, commitCount, as.Mean/1e6, as.Median/1e6, as.Min/1e6, as.Max/1e6, as.StdDevPct, statsOf(r).Mean, statsOf(begin).Mean/1e6, statsOf(commit).Mean/1e6, statsOf(update).Mean/1e6, statsOf(other).Mean/1e6, statsOf(transactionWindow).Mean/1e6, statsOf(residual).Mean/1e6)
	}
	b.WriteString("\nApply wall time and rows/s were captured with SQLSpy and diagnostic counters enabled; they include observer overhead and are not production-throughput claims. The paired uninstrumented-vs-instrumented check is recorded in `observer-overhead/observer-summary.md`. Driver event timers cover the underlying driver calls and exclude post-call event recording. The event timers are nested observations, not mutually exclusive components: `writer.Begin` wrapper timing includes schema validation/queries plus driver Begin; `WriteTx.Update` includes row validation and driver UPDATEs; driver SQL timers are measured inside those calls. Do **not** add them. Apply remainder is unassigned and includes observer work as well as page reads, masking, receipt handling, and framework overhead; it must not be attributed solely to production work. This evidence does not claim all Apply time is commit/fsync. `Commit` invocation, driver call elapsed time and successful returned Commit are separately recorded; unknown/error is never called success.\n\n")
	b.WriteString("Batch order is pseudorandomized with a fixed seed and each trial has a new DB/engine. Ten repeats are descriptive; no significance claim is made. `strace` attribution is in `fsync-evidence.md`.\n")
	return b.String()
}

type phase3EnvironmentMetadata struct {
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	Go          string `json:"go_version"`
	CPU         string `json:"cpu"`
	Cores       int    `json:"cores"`
	Kernel      string `json:"kernel"`
	MemoryTotal string `json:"memory_total_mb"`
	MemoryScope string `json:"memory_scope"`
	Driver      string `json:"sqlite_driver"`
	Repetitions int    `json:"repetitions_per_batch"`
	Samples     int    `json:"sample_count"`
	Seed        string `json:"order_seed"`
	Journal     string `json:"journal_mode"`
	Synchronous string `json:"synchronous"`
	Command     string `json:"command"`
	HeadSHA     string `json:"head_sha"`
}

func phase3Environment(samples []phase3Trial, reps int) string {
	j, s := samples[0].JournalMode, samples[0].Synchronous
	b, _ := json.MarshalIndent(phase3EnvironmentMetadata{runtime.GOOS, runtime.GOARCH, runtime.Version(), hostCPU(), runtime.NumCPU(), hostKernel(), hostMemoryTotalMB(), "host /proc/meminfo MemTotal, not cgroup/container limit", "modernc.org/sqlite v1.60.1", reps, len(samples), "0x132026", j, s, "MASK_RIVER_PHASE3_DIR=<dir> MASK_RIVER_PHASE3_REPS=10 go test -run '^TestPhase3BatchExperiment$' -count=1 -timeout 50m ./internal/runner/", currentGitSHA()}, "", "  ")
	return string(b) + "\n"
}
func hostKernel() string {
	output, err := exec.Command("uname", "-sr").Output()
	if err != nil {
		return "UNVERIFIED"
	}
	return strings.TrimSpace(string(output))
}
func hostMemoryTotalMB() string {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "UNVERIFIED"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, parseErr := strconv.ParseInt(fields[1], 10, 64)
				if parseErr == nil {
					return strconv.FormatInt(value/1024, 10)
				}
			}
		}
	}
	return "UNVERIFIED"
}

func currentGitSHA() string {
	command := exec.Command("git", "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		return "UNVERIFIED"
	}
	return strings.TrimSpace(string(output))
}

func sqliteModes(path string) (string, string) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}
	dbh, err := sql.Open("sqlite", u.String())
	if err != nil {
		return "UNVERIFIED", "UNVERIFIED"
	}
	defer dbh.Close()
	var journal string
	var syncMode int
	if dbh.QueryRow("PRAGMA journal_mode").Scan(&journal) != nil || dbh.QueryRow("PRAGMA synchronous").Scan(&syncMode) != nil {
		return "UNVERIFIED", "UNVERIFIED"
	}
	return journal, strconv.Itoa(syncMode)
}
func validatePhase3Trials(samples []phase3Trial, repetitions int) error {
	if repetitions < 1 || len(samples) != repetitions*4 {
		return fmt.Errorf("sample count %d does not match four batch sizes × %d repeats", len(samples), repetitions)
	}
	batchSizes := map[int]bool{16: true, 64: true, 128: true, 256: true}
	seenOrder, seenTrial := map[int]bool{}, map[string]bool{}
	counts := map[int]int{}
	for _, s := range samples {
		if !batchSizes[s.BatchSize] || s.Rows != 256 || s.Workers != 4 {
			return fmt.Errorf("illegal configuration batch=%d rows=%d workers=%d", s.BatchSize, s.Rows, s.Workers)
		}
		if s.Order < 1 || s.Order > len(samples) || seenOrder[s.Order] {
			return fmt.Errorf("invalid or duplicate randomized order %d", s.Order)
		}
		seenOrder[s.Order] = true
		if s.Trial < 1 || s.Trial > repetitions {
			return fmt.Errorf("batch %d has invalid repeat %d", s.BatchSize, s.Trial)
		}
		key := fmt.Sprintf("%d/%d", s.BatchSize, s.Trial)
		if seenTrial[key] {
			return fmt.Errorf("duplicate trial %s", key)
		}
		seenTrial[key] = true
		counts[s.BatchSize]++
		if s.FixtureNS <= 0 || s.WorkflowNS < s.ApplyNS || s.ApplyNS <= 0 || s.RowsCommitted != 256 || !s.Complete || !s.ValidatePassed {
			return fmt.Errorf("trial %d has invalid duration or incomplete/unvalidated apply", s.Order)
		}
		if math.IsNaN(s.RowsPerSecond) || math.IsInf(s.RowsPerSecond, 0) || s.RowsPerSecond <= 0 {
			return fmt.Errorf("trial %d has invalid throughput", s.Order)
		}
		wantRowsPerSecond := float64(s.Rows) / (float64(s.ApplyNS) / 1e9)
		if math.Abs(s.RowsPerSecond-wantRowsPerSecond) > math.Max(1, wantRowsPerSecond)*1e-12 {
			return fmt.Errorf("trial %d throughput %.12g disagrees with rows/apply %.12g", s.Order, s.RowsPerSecond, wantRowsPerSecond)
		}
		for name, ns := range map[string]int64{
			"Begin wall": s.BeginWallNS, "Commit wall": s.CommitWallNS, "Rollback wall": s.RollbackWallNS,
			"Update wall": s.UpdateCallWallNS, "driver Begin": s.DriverBeginNS, "driver Commit": s.DriverCommitNS,
			"driver Rollback": s.DriverRollbackNS, "driver UPDATE": s.UpdateExecNS, "other SQL": s.OtherSQLCallNS,
		} {
			if ns < 0 {
				return fmt.Errorf("trial %d has negative %s duration %d", s.Order, name, ns)
			}
		}
		want := (256 + s.BatchSize - 1) / s.BatchSize
		if s.BeginAttempts != want || s.BeginSucceeded != want || s.BeginFailed != 0 || s.CommitAttempts != want || s.CommitReturnedSuccess != want || s.CommitReturnedError != 0 || s.CommitUnknown != 0 || s.RollbackAttempts != 0 || s.Receipts != want {
			return fmt.Errorf("trial %d transaction/receipt counts inconsistent with %d batches", s.Order, want)
		}
		if s.UpdateStatements != 256 || len(s.BatchStatementCounts) != want || len(s.BatchUpdateCounts) != want || len(s.BatchOtherCounts) != want || len(s.BatchBeginDriverNS) != want || len(s.BatchCommitDriverNS) != want || len(s.BatchUpdateExecNS) != want || len(s.BatchOtherSQLCallNS) != want || len(s.BatchTransactionNS) != want || len(s.ReceiptRows) != want {
			return fmt.Errorf("trial %d per-batch measurement count mismatch", s.Order)
		}
		var updateCount, receiptRows int64
		var txWindow int64
		var statementCount int
		var beginNS, commitNS, updateNS, otherNS int64
		for i := 0; i < want; i++ {
			for name, ns := range map[string]int64{
				"Begin": s.BatchBeginDriverNS[i], "Commit": s.BatchCommitDriverNS[i],
				"UPDATE": s.BatchUpdateExecNS[i], "other SQL": s.BatchOtherSQLCallNS[i],
				"transaction window": s.BatchTransactionNS[i],
			} {
				if ns < 0 {
					return fmt.Errorf("trial %d batch %d has negative %s duration %d", s.Order, i+1, name, ns)
				}
			}
			if s.BatchOtherCounts[i] < 0 {
				return fmt.Errorf("trial %d batch %d has negative other statement count", s.Order, i+1)
			}
			updateCount += int64(s.BatchUpdateCounts[i])
			receiptRows += s.ReceiptRows[i]
			statementCount += s.BatchStatementCounts[i]
			beginNS += s.BatchBeginDriverNS[i]
			commitNS += s.BatchCommitDriverNS[i]
			updateNS += s.BatchUpdateExecNS[i]
			otherNS += s.BatchOtherSQLCallNS[i]
			txWindow += s.BatchTransactionNS[i]
			if s.BatchStatementCounts[i] != s.BatchUpdateCounts[i]+s.BatchOtherCounts[i]+2 || s.BatchTransactionNS[i] <= 0 || s.BatchUpdateCounts[i] <= 0 || s.ReceiptRows[i] != int64(s.BatchUpdateCounts[i]) {
				return fmt.Errorf("trial %d batch %d SQL/receipt invariant failed", s.Order, i+1)
			}
		}
		if updateCount != 256 || receiptRows != 256 || statementCount != s.Statements || beginNS != s.DriverBeginNS || commitNS != s.DriverCommitNS || updateNS != s.UpdateExecNS || otherNS != s.OtherSQLCallNS || txWindow != s.BatchTransactionSumNS || s.ApplyNS < txWindow || s.ApplyUnassignedNS != s.ApplyNS-txWindow {
			return fmt.Errorf("trial %d update/receipt/timing totals inconsistent", s.Order)
		}
		if s.CPUUserNS < -1 || s.CPUSystemNS < -1 || (s.CPUUserNS == -1) != (s.CPUSystemNS == -1) || s.RSSBeforeKB < -1 || s.RSSAfterKB < -1 || s.ReaderOpen < -1 || s.WriterOpen < -1 || s.ReaderOpenAfterClose != 0 || s.WriterOpenAfterClose != 0 {
			return fmt.Errorf("trial %d has invalid resource sentinel", s.Order)
		}
	}
	for _, batch := range []int{16, 64, 128, 256} {
		if counts[batch] != repetitions {
			return fmt.Errorf("batch %d has %d trials, want %d", batch, counts[batch], repetitions)
		}
		for i := 1; i <= repetitions; i++ {
			if !seenTrial[fmt.Sprintf("%d/%d", batch, i)] {
				return fmt.Errorf("batch %d missing trial %d", batch, i)
			}
		}
	}
	return nil
}

func phase3Resources(samples []phase3Trial) string {
	b, _ := json.MarshalIndent(samples, "", "  ")
	return string(b) + "\n"
}
func phase3SVG(samples []phase3Trial) string {
	var b strings.Builder
	b.WriteString("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"800\" height=\"300\"><text x=\"20\" y=\"20\">SQLite Apply median (synthetic, ms)</text>")
	for i, size := range []int{16, 64, 128, 256} {
		var a []float64
		for _, s := range samples {
			if s.BatchSize == size {
				a = append(a, float64(s.ApplyNS)/1e6)
			}
		}
		v := statsOf(a).Median
		fmt.Fprintf(&b, "<text x=\"20\" y=\"%d\">batch %d</text><rect x=\"100\" y=\"%d\" width=\"%.1f\" height=\"20\" fill=\"#4477aa\"/><text x=\"%d\" y=\"%d\">%.2f ms</text>", 60+i*70, size, 42+i*70, v*2, 110+int(v*2), 57+i*70, v)
	}
	b.WriteString("</svg>\n")
	return b.String()
}
func i64(v int64) string   { return strconv.FormatInt(v, 10) }
func f64(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// TestPhase3Scale10K is deliberately opt-in so it does not burden ordinary PR
// CI. It tests 10k on real SQLite with all worker counts and validates actual
// output after Apply. A single-worker output is compared row-by-row to each run.
func TestPhase3Scale10K(t *testing.T) {
	outDir := os.Getenv("MASK_RIVER_PHASE3_10K_DIR")
	if outDir == "" {
		t.Skip("set MASK_RIVER_PHASE3_10K_DIR to execute the 10k stress matrix")
	}
	root := t.TempDir()
	const rows = 10000
	type result struct {
		Workers              int     `json:"workers"`
		FixtureNS            int64   `json:"fixture_ns"`
		WorkflowNS           int64   `json:"workflow_ns"`
		ApplyNS              int64   `json:"apply_ns"`
		RowsPerSecond        float64 `json:"rows_per_second"`
		CPUUserNS            int64   `json:"cpu_user_ns"`
		CPUSystemNS          int64   `json:"cpu_system_ns"`
		RSSBeforeKB          int64   `json:"rss_before_kb"`
		RSSAfterKB           int64   `json:"rss_after_kb"`
		GoroutinesBefore     int     `json:"goroutines_before"`
		GoroutinesAfter      int     `json:"goroutines_after"`
		ReaderOpen           int     `json:"reader_open"`
		WriterOpen           int     `json:"writer_open"`
		ReaderOpenAfterClose int     `json:"reader_open_after_close"`
		WriterOpenAfterClose int     `json:"writer_open_after_close"`
		Receipts             int     `json:"receipts"`
		RowsCommitted        int64   `json:"rows_committed"`
		Complete             bool    `json:"complete"`
		ValidatePassed       bool    `json:"validate_passed"`
		SourceUnchanged      bool    `json:"source_unchanged"`
		TargetRows           int     `json:"target_rows"`
	}
	var all []result
	journalMode, syncMode := "UNVERIFIED", "UNVERIFIED"
	var baseline map[int64]string
	for _, workers := range []int{1, 2, 4, 8, 16} {
		startFixture := time.Now()
		env := benchEnvIn(t, benchConfig{rows: rows, workers: workers, batchSize: 64}, root)
		fixtureNS := time.Since(startFixture).Nanoseconds()
		if workers == 1 {
			journalMode, syncMode = sqliteModes(env.base.Target)
		}
		beforeDigest, err := fileDigest(env.base.Source)
		if err != nil {
			t.Fatal(err)
		}
		cpuU, cpuS, cpuOK := cpuNS()
		rss0, g0 := rssKB(), runtime.NumGoroutine()
		start := time.Now()
		stages, done := env.run(t)
		workflowNS := time.Since(start).Nanoseconds()
		cpuU2, cpuS2, cpuOK2 := cpuNS()
		rss1, g1 := rssKB(), runtime.NumGoroutine()
		reader, writer, poolOK := env.engine.PoolStats()
		validation, err := env.engine.Validate(context.Background(), contracts.ValidateRequest{Plan: env.lastPlan})
		if err != nil {
			t.Fatalf("workers=%d Validate: %v", workers, err)
		}
		passed := validation.Coverage.Complete && validation.Passed(true)
		actual := targetRows(t, env.base.Target)
		if len(actual) != rows || int64(done) != int64(rows) || !env.lastApply.Complete || !passed {
			t.Fatalf("workers=%d incomplete rows=%d target=%d complete=%v validate=%v", workers, done, len(actual), env.lastApply.Complete, passed)
		}
		var committed int64
		for _, rc := range env.lastApply.Committed {
			committed += rc.Rows
		}
		if committed != rows {
			t.Fatalf("workers=%d receipt rows=%d", workers, committed)
		}
		afterDigest, err := fileDigest(env.base.Source)
		if err != nil {
			t.Fatal(err)
		}
		sourceUnchanged := beforeDigest == afterDigest
		if !sourceUnchanged {
			t.Fatalf("workers=%d modified source", workers)
		}
		if workers == 1 {
			baseline = actual
		} else {
			if !sameRows(baseline, actual) {
				t.Fatalf("workers=%d produced different per-primary-key values", workers)
			}
		}
		v := result{Workers: workers, FixtureNS: fixtureNS, WorkflowNS: workflowNS, ApplyNS: stages["apply"].Nanoseconds(), RowsPerSecond: float64(rows) / (float64(workflowNS) / 1e9), CPUUserNS: -1, CPUSystemNS: -1, RSSBeforeKB: rss0, RSSAfterKB: rss1, GoroutinesBefore: g0, GoroutinesAfter: g1, Receipts: len(env.lastApply.Committed), RowsCommitted: committed, Complete: env.lastApply.Complete, ValidatePassed: passed, SourceUnchanged: sourceUnchanged, TargetRows: len(actual)}
		if cpuOK && cpuOK2 {
			v.CPUUserNS = cpuU2 - cpuU
			v.CPUSystemNS = cpuS2 - cpuS
		}
		if !poolOK {
			v.ReaderOpen = -1
			v.WriterOpen = -1
		} else {
			v.ReaderOpen = reader.OpenConnections
			v.WriterOpen = writer.OpenConnections
		}
		if err := env.close(); err != nil {
			t.Fatalf("workers=%d close engine/remove fixture: %v", workers, err)
		}
		readerClosed, writerClosed, closedOK := env.engine.PoolStats()
		if !closedOK || readerClosed.OpenConnections != 0 || writerClosed.OpenConnections != 0 {
			t.Fatalf("workers=%d pools not released after close: ok=%v reader=%d writer=%d", workers, closedOK, readerClosed.OpenConnections, writerClosed.OpenConnections)
		}
		v.ReaderOpenAfterClose, v.WriterOpenAfterClose = readerClosed.OpenConnections, writerClosed.OpenConnections
		all = append(all, v)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary 10k fixtures not released: entries=%d err=%v", len(entries), err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "10k-workers.json"), string(raw)+"\n")
	var csvText strings.Builder
	fmt.Fprintf(&csvText, "workers,fixture_ms,workflow_ms,apply_ms,workflow_rows_per_s,cpu_user_ms,cpu_system_ms,rss_before_kb,rss_after_kb,goroutines_before,goroutines_after,reader_open,writer_open,reader_open_after_close,writer_open_after_close,receipts,rows_committed,complete,validate_passed,source_unchanged,target_rows\n")
	for _, v := range all {
		fmt.Fprintf(&csvText, "%d,%.2f,%.2f,%.2f,%.2f,%s,%s,%s,%s,%d,%d,%d,%d,%d,%d,%d,%d,%t,%t,%t,%d\n", v.Workers, float64(v.FixtureNS)/1e6, float64(v.WorkflowNS)/1e6, float64(v.ApplyNS)/1e6, v.RowsPerSecond, metricMS(v.CPUUserNS), metricMS(v.CPUSystemNS), metricKB(v.RSSBeforeKB), metricKB(v.RSSAfterKB), v.GoroutinesBefore, v.GoroutinesAfter, v.ReaderOpen, v.WriterOpen, v.ReaderOpenAfterClose, v.WriterOpenAfterClose, v.Receipts, v.RowsCommitted, v.Complete, v.ValidatePassed, v.SourceUnchanged, v.TargetRows)
	}
	writeFile(t, filepath.Join(outDir, "10k-workers.csv"), csvText.String())
	meta := map[string]any{"head_sha": currentGitSHA(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "go_version": runtime.Version(), "cpu": hostCPU(), "cores": runtime.NumCPU(), "kernel": hostKernel(), "memory_total_mb": hostMemoryTotalMB(), "memory_scope": "host /proc/meminfo MemTotal, not cgroup/container limit", "sqlite_driver": "modernc.org/sqlite v1.60.1", "rows": rows, "workers": []int{1, 2, 4, 8, 16}, "iterations_per_worker": 1, "journal_mode": journalMode, "synchronous": syncMode, "fixture_scope": "separate optimized test fixture; one prepared insert transaction per database", "workflow_scope": "Scan, Plan, Preview, real Ed25519 Approve, Apply; measured separately from fixture construction", "apply_scope": "Apply stage duration within workflow", "command": "MASK_RIVER_PHASE3_10K_DIR=<dir> go test -run '^TestPhase3Scale10K$' -count=1 -timeout 20m ./internal/runner/"}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "10k-environment.json"), string(metaBytes)+"\n")
	var summary strings.Builder
	summary.WriteString("# 10,000-row SQLite Runner stress run (single observation per worker count)\n\n")
	summary.WriteString("Synthetic `@example.invalid` values; exact SHA and environment are in `10k-environment.json`. Fixture time excludes workflow; Apply is a substage of workflow. All runs used signed approval, Complete receipts, full independent Validate, unchanged source, exact row counts, and row-by-row equality to workers=1. Connections and fixture directories were released after each run. One observation/configuration is descriptive only; no significance or speedup claim.\n\n| workers | fixture ms | workflow ms | apply ms | workflow rows/s | CPU user/system ms | RSS before→after KB | goroutines before→after | open reader/writer before close | after close |\n|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, v := range all {
		fmt.Fprintf(&summary, "| %d | %.2f | %.2f | %.2f | %.0f | %s/%s | %s→%s | %d→%d | %d/%d | %d/%d |\n", v.Workers, float64(v.FixtureNS)/1e6, float64(v.WorkflowNS)/1e6, float64(v.ApplyNS)/1e6, v.RowsPerSecond, metricMS(v.CPUUserNS), metricMS(v.CPUSystemNS), metricKB(v.RSSBeforeKB), metricKB(v.RSSAfterKB), v.GoroutinesBefore, v.GoroutinesAfter, v.ReaderOpen, v.WriterOpen, v.ReaderOpenAfterClose, v.WriterOpenAfterClose)
	}
	writeFile(t, filepath.Join(outDir, "10k-summary.md"), summary.String())
}
func metricMS(value int64) string {
	if value < 0 {
		return "UNVERIFIED"
	}
	return fmt.Sprintf("%.2f", float64(value)/1e6)
}
func metricKB(value int64) string {
	if value < 0 {
		return "UNVERIFIED"
	}
	return strconv.FormatInt(value, 10)
}

func sameRows(a, b map[int64]string) bool {
	if len(a) != len(b) {
		return false
	}
	keys := make([]int64, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}
