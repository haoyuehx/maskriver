package main

// Real CLI subprocess scale evidence for the streaming path.
//
// Opt-in: MASKRIVER_WP3F_CLI_SCALE=1 and MASKRIVER_WP3F_SCALE_DIR=<dir>.
// Each sample builds a fresh synthetic source, copies it to the target, runs the
// real CLI child process for scan, preview, approve, apply and validate, and
// records wall time and the child's /proc VmHWM. Fixture generation and copying
// are outside every timed phase.

import (
	"bufio"
	"database/sql"
	"encoding/csv"
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

const (
	maxScaleRows      = 5_000_000
	maxScaleRepeats   = 10
	maxScaleTotalRows = 40_000_000
)

type scaleSample struct {
	rows    int
	workers int
	batch   int
	repeat  int
}

func TestCLIStreamingScaleEvidence(t *testing.T) {
	requireLinuxOutbox(t)
	if os.Getenv("MASKRIVER_WP3F_CLI_SCALE") != "1" {
		t.Skip("set MASKRIVER_WP3F_CLI_SCALE=1 to run CLI scale evidence")
	}
	outDir := os.Getenv("MASKRIVER_WP3F_SCALE_DIR")
	if outDir == "" || !filepath.IsAbs(outDir) || filepath.Clean(outDir) == "/" {
		t.Fatal("MASKRIVER_WP3F_SCALE_DIR must be an explicit absolute non-root directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rowsList := []int{10000}
	if raw := os.Getenv("MASKRIVER_WP3F_CLI_ROWS"); raw != "" {
		rowsList = nil
		for _, part := range strings.Split(raw, ",") {
			value, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || value <= 0 || value > maxScaleRows {
				t.Fatalf("invalid rows list %q (1..%d)", raw, maxScaleRows)
			}
			rowsList = append(rowsList, value)
		}
	}
	repeats := 5
	if raw := os.Getenv("MASKRIVER_WP3F_CLI_REPEATS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > maxScaleRepeats {
			t.Fatalf("invalid repeats %q (1..%d)", raw, maxScaleRepeats)
		}
		repeats = value
	}
	cells := []struct{ workers, batch int }{{1, 64}, {4, 64}, {4, 256}}
	totalSamples := 0
	for _, rows := range rowsList {
		totalSamples += rows * len(cells) * minInt(repeats, 3)
	}
	if totalSamples > maxScaleTotalRows {
		t.Fatalf("requested matrix too large: %d sample-rows (limit %d)", totalSamples, maxScaleTotalRows)
	}
	file, err := os.Create(filepath.Join(outDir, "cli-streaming-scale.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{
		"rows", "workers", "batch_size", "repeat", "scan_ms", "preview_ms",
		"approve_ms", "apply_ms", "validate_ms", "total_ms", "apply_peak_rss_kb",
		"rows_committed", "batches_committed", "outbox_bytes",
	}); err != nil {
		t.Fatal(err)
	}
	for _, rows := range rowsList {
		f := newScaleFixture(t, rows)
		defer f.cleanup()
		budget := f.budgetFile(t, outDir)
		for _, cell := range cells {
			reps := repeats
			if rows >= 1000000 {
				reps = minInt(reps, 3)
			} else if rows >= 100000 {
				reps = minInt(reps, 3)
			}
			for repeat := 0; repeat < reps; repeat++ {
				record := f.runSample(t, scaleSample{rows: rows, workers: cell.workers, batch: cell.batch, repeat: repeat}, budget)
				if err := writer.Write(record); err != nil {
					t.Fatal(err)
				}
				writer.Flush()
			}
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type scaleFixture struct {
	dir     string
	source  string
	base    string
	key     string
	trust   string
	hist    string
	priv    string
	lastRSS int64
}

func (f *scaleFixture) cleanup() {}

// newScaleFixture builds one synthetic 64-distinct-value dataset. The target is
// recreated from the pristine source for every timed sample.
func newScaleFixture(t *testing.T, rows int) *scaleFixture {
	t.Helper()
	dir := t.TempDir()
	f := &scaleFixture{
		dir: dir, source: filepath.Join(dir, "source.db"), base: filepath.Join(dir, "base.db"),
		key: filepath.Join(dir, "masking.key"),
	}
	seedScaleDataset(t, f.base, rows)
	raw, err := os.ReadFile(f.base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.source, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.key, []byte(strings.Repeat("K", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	provisioning := newReviewProvisioning(t)
	f.trust, f.hist, f.priv = provisioning.trustFile, provisioning.historyDir, provisioning.reviewerKeyFile
	return f
}

func seedScaleDataset(t *testing.T, path string, rows int) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=journal_mode(delete)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO people (id, email) VALUES (?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := statement.Exec(i+1, fmt.Sprintf("user%02d@example.invalid", i%64)); err != nil {
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

func (f *scaleFixture) budgetFile(t *testing.T, outDir string) string {
	t.Helper()
	path := filepath.Join(outDir, "budget.json")
	content := `{
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
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *scaleFixture) runSample(t *testing.T, sample scaleSample, budget string) []string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "target.db")
	raw, err := os.ReadFile(f.source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	runID := fmt.Sprintf("%032x", 0x200000+sample.rows%1000*10+sample.repeat)
	scanArgs := []string{
		"--source", f.source, "--target", target, "--min-samples", "1",
		"--scope", "people.email",
	}
	scanMS, _ := f.timed(t, append([]string{"scan"}, scanArgs...)...)
	planArgs := []string{
		"--source", f.source, "--target", target,
		"--strategy", "fake_email", "--key-id", "synthetic-key",
		"--min-samples", "1", "--scope", "people.email",
		"--batch-size", strconv.Itoa(sample.batch), "--workers", strconv.Itoa(sample.workers),
	}
	previewMS, _ := f.timed(t, append(append([]string{"mask"}, planArgs...), "--key-file", f.key)...)
	approveArgs := append([]string{"approve"}, planArgs...)
	approveArgs = append(approveArgs, "--trust-file", f.trust, "--history-dir", f.hist,
		"--reviewer-key-id", "reviewer-1", "--reviewer-key-file", f.priv,
		"--reviewer-id", "reviewer@synthetic.example", "--streaming")
	approveMS, _ := f.timed(t, approveArgs...)
	initArgs := []string{"init-outbox", "--target", target}
	if code, out := runCLIProcess(t, initArgs...); code != 0 {
		t.Fatalf("init-outbox failed: %s", out)
	}
	applyArgs := append([]string{"mask"}, planArgs...)
	applyArgs = append(applyArgs, "--key-file", f.key, "--trust-file", f.trust,
		"--history-dir", f.hist, "--approved", "--apply", "--streaming",
		"--budget-file", budget, "--run-id", runID, "--receipt-page-size", "8",
		"--timeout", "30m")
	applyMS, applyOut := f.timed(t, applyArgs...)
	applyPeakRSS := f.lastRSS
	validateMS, _ := f.timed(t, f.validateArgsFor(target, sample)...)
	rowsCommitted := fieldInt(applyOut, "rows_committed=")
	batches := fieldInt(applyOut, "batches_committed=")
	info, _ := os.Stat(target)
	outbox := int64(0)
	if info != nil {
		outbox = info.Size()
	}
	return []string{
		strconv.Itoa(sample.rows), strconv.Itoa(sample.workers), strconv.Itoa(sample.batch),
		strconv.Itoa(sample.repeat), strconv.FormatInt(scanMS, 10), strconv.FormatInt(previewMS, 10),
		strconv.FormatInt(approveMS, 10), strconv.FormatInt(applyMS, 10),
		strconv.FormatInt(validateMS, 10), strconv.FormatInt(scanMS+previewMS+approveMS+applyMS+validateMS, 10),
		strconv.FormatInt(applyPeakRSS, 10), strconv.FormatInt(rowsCommitted, 10),
		strconv.FormatInt(batches, 10), strconv.FormatInt(outbox, 10),
	}
}

func (f *scaleFixture) validateArgsFor(target string, sample scaleSample) []string {
	return []string{"validate", "--source", f.source, "--target", target,
		"--strategy", "fake_email", "--key-id", "synthetic-key", "--min-samples", "1",
		"--scope", "people.email", "--key-file", f.key, "--strict"}
}

// timed runs one child process and samples its VmHWM while it runs.
func (f *scaleFixture) timed(t *testing.T, args ...string) (int64, string) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcessChild$", "--"}, args...)...)
	command.Env = append(os.Environ(), cliChildEnv+"=1")
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
	elapsed := time.Since(start)
	mu.Lock()
	f.lastRSS = peak
	mu.Unlock()
	if waitErr != nil {
		t.Fatalf("%v exited with %v: %s", args[0], waitErr, output.String())
	}
	return elapsed.Milliseconds(), output.String()
}

// readVmHWM returns the process high-water RSS in KiB from /proc. It is only
// available on Linux and returns 0 elsewhere.
func readVmHWM(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return value
				}
			}
		}
	}
	return 0
}

// fieldInt extracts the integer after key from one sanitized summary line.
func fieldInt(output, key string) int64 {
	for _, line := range strings.Split(output, "\n") {
		if index := strings.Index(line, key); index >= 0 {
			rest := line[index+len(key):]
			end := 0
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			value, err := strconv.ParseInt(rest[:end], 10, 64)
			if err == nil {
				return value
			}
		}
	}
	return -1
}
