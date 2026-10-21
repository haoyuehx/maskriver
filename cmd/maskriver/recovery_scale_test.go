package main

// Opt-in real-CLI scale evidence for Issue #14 recovery.
//
// Run with:
//   MASKRIVER_ISSUE14_SCALE=1 MASKRIVER_ISSUE14_SCALE_DIR=/abs/dir \
//     MASKRIVER_ISSUE14_ROWS=10000,100000 go test -run TestCLIRecoveryScaleEvidence -v ./cmd/maskriver
//
// The interrupted run kills the real child process after a deterministic number
// of committed batches; the resume is a separate child process on the same
// files. The baseline is a separate uninterrupted run on an identical dataset.

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCLIRecoveryScaleEvidence(t *testing.T) {
	requireLinuxOutbox(t)
	if os.Getenv("MASKRIVER_ISSUE14_SCALE") != "1" {
		t.Skip("set MASKRIVER_ISSUE14_SCALE=1 to run recovery scale evidence")
	}
	outDir := os.Getenv("MASKRIVER_ISSUE14_SCALE_DIR")
	if outDir == "" || !filepath.IsAbs(outDir) || filepath.Clean(outDir) == "/" {
		t.Fatal("MASKRIVER_ISSUE14_SCALE_DIR must be an explicit absolute non-root directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rowsList := []int{10000, 100000}
	if raw := os.Getenv("MASKRIVER_ISSUE14_ROWS"); raw != "" {
		rowsList = nil
		for _, part := range strings.Split(raw, ",") {
			value, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || value <= 0 || value > maxScaleRows {
				t.Fatalf("invalid rows list %q", raw)
			}
			rowsList = append(rowsList, value)
		}
	}
	file, err := os.Create(filepath.Join(outDir, "cli-recovery-scale.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{
		"rows", "scenario", "interrupt_after_batches", "baseline_apply_ms",
		"interrupted_apply_ms", "resume_ms", "resume_overhead_ratio", "validate_ms",
		"total_ms", "interrupted_peak_rss_kb", "resume_peak_rss_kb", "rows_committed",
		"batches_committed", "checkpoint_sequence", "progress_rows", "receipt_rows",
		"target_bytes", "digest_matches_baseline", "strict_validate",
	}); err != nil {
		t.Fatal(err)
	}
	for _, rows := range rowsList {
		f := newScaleFixture(t, rows)
		defer f.cleanup()
		batch := 256
		batches := (rows + batch - 1) / batch
		interruptAt := batches / 2
		if interruptAt < 1 {
			interruptAt = 1
		}

		baselineTarget := filepath.Join(t.TempDir(), "baseline.db")
		copyFileForScale(t, f.source, baselineTarget)
		baselineRunID := fmt.Sprintf("%032x", 0x140000+rows%1000)
		f.initRecoveryForScale(t, baselineTarget)
		baselineArgs := f.scalePlanArgs(baselineTarget, batch, baselineRunID)
		f.approveScale(t, baselineArgs, baselineTarget)
		baselineApplyMS, _ := f.timed(t, append([]string{"mask"}, baselineArgs...)...)
		baselineValidateMS, baselineValidateOut := f.timed(t, f.scaleValidateArgs(baselineTarget)...)
		baselineDigest := emailDigest(t, baselineTarget)
		baselineStats := recoveryStats(t, baselineTarget, baselineRunID)

		interruptedTarget := filepath.Join(t.TempDir(), "interrupted.db")
		copyFileForScale(t, f.source, interruptedTarget)
		interruptedRunID := fmt.Sprintf("%032x", 0x150000+rows%1000)
		f.initRecoveryForScale(t, interruptedTarget)
		interruptedArgs := f.scalePlanArgs(interruptedTarget, batch, interruptedRunID)
		f.approveScale(t, interruptedArgs, interruptedTarget)
		interruptedMS, _, interruptedRSS := f.timedFault(t, fmt.Sprintf("after_commit=%d", interruptAt),
			append([]string{"mask"}, interruptedArgs...)...)
		resumeMS, resumeOut, resumeRSS := f.timedFault(t, "",
			append(append([]string{"mask"}, interruptedArgs...), "--resume")...)
		resumeValidateMS, resumeValidateOut := f.timed(t, f.scaleValidateArgs(interruptedTarget)...)
		resumedDigest := emailDigest(t, interruptedTarget)
		stats := recoveryStats(t, interruptedTarget, interruptedRunID)
		rowsCommitted := fieldInt(resumeOut, "rows_committed=")
		batchesCommitted := fieldInt(resumeOut, "batches_committed=")
		info, _ := os.Stat(interruptedTarget)
		var targetBytes int64
		if info != nil {
			targetBytes = info.Size()
		}
		ratio := 0.0
		if baselineApplyMS > 0 {
			ratio = float64(resumeMS) / float64(baselineApplyMS)
		}
		record := []string{
			strconv.Itoa(rows), "baseline", "", strconv.FormatInt(baselineApplyMS, 10),
			"", "", "", strconv.FormatInt(baselineValidateMS, 10),
			strconv.FormatInt(baselineApplyMS+baselineValidateMS, 10), "", "",
			strconv.FormatInt(baselineStats.rowsCommitted, 10),
			strconv.FormatInt(baselineStats.batches, 10),
			strconv.FormatInt(baselineStats.checkpoint, 10),
			strconv.FormatInt(baselineStats.progressRows, 10),
			strconv.FormatInt(baselineStats.receiptRows, 10),
			"", "", strconv.FormatBool(strings.Contains(baselineValidateOut, "pass=true")),
		}
		if err := writer.Write(record); err != nil {
			t.Fatal(err)
		}
		writer.Flush()
		record = []string{
			strconv.Itoa(rows), "interrupted+resume", strconv.Itoa(interruptAt),
			strconv.FormatInt(baselineApplyMS, 10), strconv.FormatInt(interruptedMS, 10),
			strconv.FormatInt(resumeMS, 10), fmt.Sprintf("%.3f", ratio),
			strconv.FormatInt(resumeValidateMS, 10),
			strconv.FormatInt(interruptedMS+resumeMS+resumeValidateMS, 10),
			strconv.FormatInt(interruptedRSS, 10), strconv.FormatInt(resumeRSS, 10),
			strconv.FormatInt(rowsCommitted, 10), strconv.FormatInt(batchesCommitted, 10),
			strconv.FormatInt(stats.checkpoint, 10), strconv.FormatInt(stats.progressRows, 10),
			strconv.FormatInt(stats.receiptRows, 10), strconv.FormatInt(targetBytes, 10),
			strconv.FormatBool(baselineDigest == resumedDigest),
			strconv.FormatBool(strings.Contains(resumeValidateOut, "pass=true")),
		}
		if err := writer.Write(record); err != nil {
			t.Fatal(err)
		}
		writer.Flush()
	}
}

type scaleRecoveryStats struct {
	rowsCommitted, batches, checkpoint, progressRows, receiptRows int64
}

func recoveryStats(t *testing.T, path, runID string) scaleRecoveryStats {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var stats scaleRecoveryStats
	if err := database.QueryRow(`SELECT checkpoint_sequence FROM maskriver_recovery_manifest WHERE run_id=?`, runID).
		Scan(&stats.checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM maskriver_recovery_progress WHERE run_id=?`, runID).
		Scan(&stats.progressRows); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*), COALESCE(SUM(affected_rows),0) FROM maskriver_outbox_batch WHERE run_id=?`, runID).
		Scan(&stats.receiptRows, &stats.rowsCommitted); err != nil {
		t.Fatal(err)
	}
	stats.batches = stats.receiptRows
	return stats
}

// emailDigest streams the ordered id/email pairs and hashes them, so the A/B
// comparison never materializes the table.
func emailDigest(t *testing.T, path string) string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`SELECT id, email FROM people ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%d\x00%s\x00", id, email)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func copyFileForScale(t *testing.T, source, target string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *scaleFixture) initRecoveryForScale(t *testing.T, target string) {
	t.Helper()
	// Recovery is additive to an existing outbox v1 schema.
	if code, out := runCLIProcess(t, "init-outbox", "--target", target); code != 0 {
		t.Fatalf("init-outbox failed: %s", out)
	}
	if code, out := runCLIProcess(t, "init-recovery", "--target", target); code != 0 {
		t.Fatalf("init-recovery failed: %s", out)
	}
}

func (f *scaleFixture) scalePlanArgs(target string, batch int, runID string) []string {
	return []string{
		"--source", f.source, "--target", target,
		"--strategy", "fake_email", "--key-id", "synthetic-key",
		"--min-samples", "1", "--sample-limit", "10000", "--scope", "people.email",
		"--batch-size", strconv.Itoa(batch), "--workers", "4",
		"--key-file", f.key, "--trust-file", f.trust, "--history-dir", f.hist,
		"--approved", "--apply", "--streaming",
		"--budget-file", filepath.Join(f.dir, "budget.json"),
		"--run-id", runID, "--receipt-page-size", "8", "--timeout", "30m",
	}
}

func (f *scaleFixture) approveScale(t *testing.T, planArgs []string, target string) {
	t.Helper()
	// The plan args reference f.dir, so write the same explicit budget there.
	if err := os.WriteFile(filepath.Join(f.dir, "budget.json"), []byte(scaleBudgetJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	// Rebuild explicitly rather than filtering apply-only flags.
	args := []string{"approve",
		"--source", f.source, "--target", target,
		"--strategy", "fake_email", "--key-id", "synthetic-key",
		"--min-samples", "1", "--sample-limit", "10000", "--scope", "people.email",
		"--trust-file", f.trust, "--history-dir", f.hist,
		"--reviewer-key-id", "reviewer-1", "--reviewer-key-file", f.priv,
		"--reviewer-id", "reviewer@synthetic.example", "--streaming"}
	_ = planArgs
	if code, out := runCLIProcess(t, args...); code != 0 {
		t.Fatalf("approve failed: %s", out)
	}
}

func (f *scaleFixture) scaleValidateArgs(target string) []string {
	return []string{"validate", "--source", f.source, "--target", target,
		"--strategy", "fake_email", "--key-id", "synthetic-key", "--min-samples", "1",
		"--sample-limit", "10000", "--scope", "people.email", "--key-file", f.key, "--strict"}
}

const scaleBudgetJSON = `{
  "MaxBatchSize": 256,
  "MaxWorkers": 16,
  "MaxRowBytes": 1048576,
  "MaxColumns": 64,
  "MaxTables": 8,
  "MaxHistoryEntries": 512,
  "MaxHistoryNameBytes": 4096,
  "MaxReceiptPage": 128,
  "MaxIssueCounts": 32,
  "MaxQueue": 0,
  "MaxProcessBytes": 2147483648,
  "MaxDiskBytes": 8589934592
}`

// timedFault runs one CLI child with an optional fault specification and returns
// wall time, output and peak RSS. A non-empty fault is expected to exit 70.
func newScaleCommand(args ...string) *exec.Cmd {
	return exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcessChild$", "--"}, args...)...)
}

func exitCodeOf(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (f *scaleFixture) timedFault(t *testing.T, fault string, args ...string) (int64, string, int64) {
	t.Helper()
	command := newScaleCommand(args...)
	command.Env = append(command.Env, cliChildEnv+"=1")
	if fault != "" {
		command.Env = append(command.Env, cliFaultEnv+"="+fault)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = command.Stdout
	start := time.Now()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	var mu sync.Mutex
	peak := int64(0)
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		status := fmt.Sprintf("/proc/%d/status", pid)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if rss := readVmHWM(status); rss > 0 {
				mu.Lock()
				if rss > peak {
					peak = rss
				}
				mu.Unlock()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	scanner := bufio.NewScanner(stdout)
	var output strings.Builder
	for scanner.Scan() {
		output.WriteString(scanner.Text())
		output.WriteByte('\n')
	}
	waitErr := command.Wait()
	close(stop)
	<-finished
	elapsed := time.Since(start).Milliseconds()
	mu.Lock()
	f.lastRSS = peak
	mu.Unlock()
	if fault != "" {
		if waitErr == nil {
			t.Fatalf("fault %s did not interrupt the process", fault)
		}
		// Exit code 70 is the deterministic fault exit.
		if code := exitCodeOf(waitErr); code != 70 {
			t.Fatalf("fault %s exit=%d, want 70; output=%s", fault, code, output.String())
		}
	} else if waitErr != nil {
		t.Fatalf("%v exited with %v: %s", args[0], waitErr, output.String())
	}
	return elapsed, output.String(), peak
}
