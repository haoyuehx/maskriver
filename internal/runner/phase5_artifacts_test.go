package runner

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var phase5RollupHeader = []string{
	"rows", "workers", "repetitions", "workflow_median_ns", "workflow_min_ns", "workflow_max_ns",
	"apply_median_ns", "apply_min_ns", "apply_max_ns", "rows_per_second_median", "peak_rss_max_kb_sampled",
	"peak_heap_alloc_max_bytes_sampled", "receipt_count", "receipt_capacity_bytes_max", "successful_commit_count",
	"source_unchanged_all", "validate_passed_all", "null_counts_match_all",
}

func TestPhase5ArchivedArtifactConsistency(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "..", "..", "docs", "perf", "issue13-phase5-20261019-final")
	var samples []phase5Sample
	if err := readPhase5JSON(filepath.Join(dir, "samples.json"), &samples); err != nil {
		t.Fatal(err)
	}
	matrix := phase5Matrix()
	if err := validatePhase5Matrix(samples, matrix); err != nil {
		t.Fatalf("archived matrix invalid: %v", err)
	}
	for _, sample := range samples {
		if err := validatePhase5Sample(sample, phase5Case{Rows: sample.Rows, Workers: sample.Workers, Repetition: sample.Repetition}); err != nil {
			t.Fatalf("archived sample %s invalid: %v", sample.TrialID, err)
		}
		var raw phase5Sample
		if err := readPhase5JSON(filepath.Join(dir, "raw", sample.TrialID, "sample.json"), &raw); err != nil {
			t.Fatalf("raw sample %s: %v", sample.TrialID, err)
		}
		if !reflect.DeepEqual(raw, sample) {
			t.Fatalf("raw and aggregate sample differ for %s", sample.TrialID)
		}
	}
	if err := validatePhase5CrossWorker(samples); err != nil {
		t.Fatalf("archived cross-worker output mismatch: %v", err)
	}

	var matrixArtifact struct {
		Completed int          `json:"completed"`
		Cases     []phase5Case `json:"cases"`
	}
	if err := readPhase5JSON(filepath.Join(dir, "matrix.json"), &matrixArtifact); err != nil {
		t.Fatal(err)
	}
	if matrixArtifact.Completed != len(samples) || !reflect.DeepEqual(matrixArtifact.Cases, matrix) {
		t.Fatalf("matrix metadata disagrees with %d expected cases", len(samples))
	}

	ndjson, err := os.Open(filepath.Join(dir, "samples.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer ndjson.Close()
	var lines []phase5Sample
	scanner := bufio.NewScanner(ndjson)
	for scanner.Scan() {
		var sample phase5Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			t.Fatalf("invalid NDJSON sample: %v", err)
		}
		lines = append(lines, sample)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, samples) {
		t.Fatal("samples.ndjson content differs from samples.json")
	}

	resourceRecords, err := readPhase5ResourceRecordSet(samples, dir)
	if err != nil {
		t.Fatal(err)
	}
	aggregateResources, err := readPhase5CSVRecords(filepath.Join(dir, "resource-samples.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase5ResourceRecords(resourceRecords, aggregateResources); err != nil {
		t.Fatalf("aggregate resource samples differ from raw trial samples: %v", err)
	}
	parsedResources, err := readPhase5Resources(filepath.Join(dir, "resource-samples.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsedResources) != len(aggregateResources)-1 {
		t.Fatal("aggregate resource sample count mismatch")
	}

	rollup, err := readPhase5CSVRecords(filepath.Join(dir, "rollup.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase5Rollup(samples, rollup); err != nil {
		t.Fatalf("rollup.csv disagrees with samples.json: %v", err)
	}

	summary, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase5Summary(samples, len(parsedResources), string(summary)); err != nil {
		t.Fatalf("summary.md disagrees with raw samples: %v", err)
	}
}

func TestPhase5ArchivedArtifactValidatorsRejectMetricMutation(t *testing.T) {
	dir := phase5ArtifactDir(t)
	var samples []phase5Sample
	if err := readPhase5JSON(filepath.Join(dir, "samples.json"), &samples); err != nil {
		t.Fatal(err)
	}
	badSamples := append([]phase5Sample(nil), samples...)
	badSamples[0].ApplyNS++
	if reflect.DeepEqual(samples[0], badSamples[0]) {
		t.Fatal("test mutation did not change sample")
	}
	if phase5SampleEqual(samples[0], badSamples[0]) {
		t.Fatal("whole-sample comparator accepted a changed Apply duration")
	}

	rollup, err := readPhase5CSVRecords(filepath.Join(dir, "rollup.csv"))
	if err != nil {
		t.Fatal(err)
	}
	mutatedRollup := clonePhase5CSVRecords(rollup)
	mutatedRollup[1][9] = "0"
	if err := validatePhase5Rollup(samples, mutatedRollup); err == nil {
		t.Fatal("rollup validator accepted a mutated rows/s value")
	}

	resources, err := readPhase5ResourceRecordSet(samples, dir)
	if err != nil {
		t.Fatal(err)
	}
	mutatedResources := clonePhase5CSVRecords(resources)
	mutatedResources[1][2] = "0"
	if err := validatePhase5ResourceRecords(resources, mutatedResources); err == nil {
		t.Fatal("resource validator accepted a mutated RSS sample")
	}

	summary, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	mutatedSummary := strings.Replace(string(summary), "4,716", "4,715", 1)
	if mutatedSummary == string(summary) {
		t.Fatal("test mutation did not find the expected summary metric")
	}
	resourceRecords, err := readPhase5ResourceRecordSet(samples, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase5Summary(samples, len(resourceRecords)-1, mutatedSummary); err == nil {
		t.Fatal("summary validator accepted a mutated rows/s value")
	}
	mutatedReceiptSummary := strings.Replace(string(summary), ", 0.133 MiB at 100k, and", ", 0.134 MiB at 100k, and", 1)
	if mutatedReceiptSummary == string(summary) {
		t.Fatal("test mutation did not find the expected receipt-capacity statement")
	}
	if err := validatePhase5Summary(samples, len(resourceRecords)-1, mutatedReceiptSummary); err == nil {
		t.Fatal("summary validator accepted a mutated receipt-capacity statement")
	}
}

func phase5ArtifactDir(t *testing.T) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(source), "..", "..", "docs", "perf", "issue13-phase5-20261019-final")
}

func readPhase5JSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func readPhase5CSVRecords(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	return reader.ReadAll()
}

func readPhase5ResourceRecordSet(samples []phase5Sample, dir string) ([][]string, error) {
	var expected [][]string
	for _, sample := range samples {
		path := filepath.Join(dir, "raw", sample.TrialID, "resource-samples.csv")
		records, err := readPhase5CSVRecords(path)
		if err != nil {
			return nil, fmt.Errorf("read resource records for %s: %w", sample.TrialID, err)
		}
		if len(records) < 2 || len(records[0]) != 17 || records[0][0] != "trial_id" {
			return nil, fmt.Errorf("invalid resource CSV for %s", sample.TrialID)
		}
		if expected == nil {
			expected = append(expected, append([]string(nil), records[0]...))
		} else if !reflect.DeepEqual(expected[0], records[0]) {
			return nil, fmt.Errorf("resource CSV header mismatch for %s", sample.TrialID)
		}
		parsed, err := readPhase5Resources(path)
		if err != nil {
			return nil, fmt.Errorf("parse resource records for %s: %w", sample.TrialID, err)
		}
		if len(parsed) != sample.ResourceSampleCount {
			return nil, fmt.Errorf("resource sample count mismatch for %s: got %d want %d", sample.TrialID, len(parsed), sample.ResourceSampleCount)
		}
		peakRSS, peakAlloc, peakInuse, peakGoroutines, peakFDs, peakPause := phase5Peaks(parsed)
		if peakRSS != sample.PeakRSSKB || peakAlloc != sample.PeakHeapAllocBytes || peakInuse != sample.PeakHeapInuseBytes || peakGoroutines != sample.PeakGoroutines || peakFDs != sample.PeakFDs || peakPause != sample.PeakGCPauseTotalNS {
			return nil, fmt.Errorf("resource peaks disagree with sample %s", sample.TrialID)
		}
		for _, record := range records[1:] {
			if len(record) != 17 || record[0] != sample.TrialID {
				return nil, fmt.Errorf("invalid resource row for %s", sample.TrialID)
			}
			expected = append(expected, append([]string(nil), record...))
		}
	}
	return expected, nil
}

func validatePhase5ResourceRecords(expected, actual [][]string) error {
	if !reflect.DeepEqual(expected, actual) {
		return errors.New("resource CSV records differ")
	}
	return nil
}

func validatePhase5Rollup(samples []phase5Sample, records [][]string) error {
	expected := phase5ExpectedRollupRecords(samples)
	if len(records) != len(expected) {
		return fmt.Errorf("rollup record count %d, want %d", len(records), len(expected))
	}
	if !reflect.DeepEqual(records[0], phase5RollupHeader) {
		return errors.New("rollup header mismatch")
	}
	for rowIndex := 1; rowIndex < len(expected); rowIndex++ {
		want, got := expected[rowIndex], records[rowIndex]
		if len(got) != len(want) {
			return fmt.Errorf("rollup row %d has %d fields, want %d", rowIndex, len(got), len(want))
		}
		for column, expectedValue := range want {
			actualValue := got[column]
			if column >= 15 {
				if !strings.EqualFold(actualValue, expectedValue) {
					return fmt.Errorf("rollup boolean mismatch at row %d column %d", rowIndex, column)
				}
			} else if actualValue != expectedValue {
				return fmt.Errorf("rollup mismatch at row %d column %d", rowIndex, column)
			}
		}
	}
	return nil
}

func phase5ExpectedRollupRecords(samples []phase5Sample) [][]string {
	groups := make(map[[2]int][]phase5Sample)
	for _, sample := range samples {
		key := [2]int{sample.Rows, sample.Workers}
		groups[key] = append(groups[key], sample)
	}
	keys := make([][2]int, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] == keys[j][0] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})
	rows := [][]string{append([]string(nil), phase5RollupHeader...)}
	for _, key := range keys {
		group := groups[key]
		workflow, apply, rps := make([]int64, 0, len(group)), make([]int64, 0, len(group)), make([]float64, 0, len(group))
		for _, sample := range group {
			workflow = append(workflow, sample.WorkflowNS)
			apply = append(apply, sample.ApplyNS)
			rps = append(rps, sample.RowsPerSecond)
		}
		row := []string{
			strconv.Itoa(key[0]), strconv.Itoa(key[1]), strconv.Itoa(len(group)),
			strconv.FormatInt(phase5MedianInt64(workflow), 10), strconv.FormatInt(phase5MinInt64(workflow), 10), strconv.FormatInt(phase5MaxInt64(workflow), 10),
			strconv.FormatInt(phase5MedianInt64(apply), 10), strconv.FormatInt(phase5MinInt64(apply), 10), strconv.FormatInt(phase5MaxInt64(apply), 10),
			strconv.FormatFloat(math.Round(phase5MedianFloat64(rps)*1000)/1000, 'f', 3, 64),
			strconv.FormatInt(phase5MaxInt64(phase5Ints64(group, func(s phase5Sample) int64 { return s.PeakRSSKB })), 10),
			strconv.FormatUint(phase5MaxUint64(group, func(s phase5Sample) uint64 { return s.PeakHeapAllocBytes }), 10),
			strconv.Itoa(group[0].ReceiptCount), strconv.FormatUint(uint64(phase5MaxInt64(phase5Ints64(group, func(s phase5Sample) int64 { return int64(s.ReceiptSliceCapacityBytes) }))), 10),
			strconv.Itoa(group[0].SuccessfulCommitCount),
			phase5BoolString(phase5Every(group, func(s phase5Sample) bool { return s.SourceFileSHA256Before == s.SourceFileSHA256After })),
			phase5BoolString(phase5Every(group, func(s phase5Sample) bool { return s.ValidatePassed })),
			phase5BoolString(phase5Every(group, func(s phase5Sample) bool { return s.TargetNullCountsMatch })),
		}
		rows = append(rows, row)
	}
	return rows
}

func phase5MedianInt64(values []int64) int64 {
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	if len(copyValues)%2 == 1 {
		return copyValues[len(copyValues)/2]
	}
	return (copyValues[len(copyValues)/2-1] + copyValues[len(copyValues)/2]) / 2
}

func phase5MedianFloat64(values []float64) float64 {
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	if len(copyValues)%2 == 1 {
		return copyValues[len(copyValues)/2]
	}
	return (copyValues[len(copyValues)/2-1] + copyValues[len(copyValues)/2]) / 2
}

func phase5MinInt64(values []int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func phase5MaxInt64(values []int64) int64 {
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func phase5Ints64(samples []phase5Sample, value func(phase5Sample) int64) []int64 {
	out := make([]int64, len(samples))
	for i, sample := range samples {
		out[i] = value(sample)
	}
	return out
}

func phase5MaxUint64(samples []phase5Sample, value func(phase5Sample) uint64) uint64 {
	var maximum uint64
	for _, sample := range samples {
		if current := value(sample); current > maximum {
			maximum = current
		}
	}
	return maximum
}

func phase5Every(samples []phase5Sample, predicate func(phase5Sample) bool) bool {
	for _, sample := range samples {
		if !predicate(sample) {
			return false
		}
	}
	return len(samples) > 0
}

func phase5BoolString(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func validatePhase5Summary(samples []phase5Sample, resourceCount int, summary string) error {
	var missing []string
	for _, row := range phase5SummaryResultRows(samples) {
		if !strings.Contains(summary, row) {
			missing = append(missing, "result: "+row)
		}
	}
	for _, row := range phase5SummaryStageRows(samples) {
		if !strings.Contains(summary, row) {
			missing = append(missing, "stage: "+row)
		}
	}
	for _, row := range phase5SummaryCPURows(samples) {
		if !strings.Contains(summary, row) {
			missing = append(missing, "CPU: "+row)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("summary metric rows disagree with samples: %s", strings.Join(missing, "; "))
	}
	peakRSS, peakHeap, _, peakGoroutines, peakFDs := phase5ResourceSummary(samples)
	resourceStatement := fmt.Sprintf("The sampler recorded %s resource rows. Maximum sampled RSS was %.1f MiB, sampled heap allocation %.2f MiB, sampled goroutines %d, and sampled file descriptors %d.", phase5CommaInt(int64(resourceCount)), float64(peakRSS)/1024, float64(peakHeap)/1048576, peakGoroutines, peakFDs)
	if !strings.Contains(summary, resourceStatement) {
		return errors.New("summary resource maxima/count disagree with archived samples")
	}
	if !strings.Contains(summary, fmt.Sprintf("All %s trials completed, passed Validate", phase5CommaInt(int64(len(samples))))) {
		return errors.New("summary trial count/completion statement is inconsistent")
	}
	receiptStatement, err := phase5ReceiptCapacityStatement(samples)
	if err != nil {
		return err
	}
	if !strings.Contains(summary, receiptStatement) {
		return errors.New("summary receipt-capacity statement disagrees with archived samples")
	}
	return nil
}

func phase5ReceiptCapacityStatement(samples []phase5Sample) (string, error) {
	capacities := make(map[int]uint64)
	for _, sample := range samples {
		if uint64(sample.ReceiptSliceCapacityBytes) > capacities[sample.Rows] {
			capacities[sample.Rows] = uint64(sample.ReceiptSliceCapacityBytes)
		}
	}
	for _, rows := range []int{10_000, 100_000, 1_000_000} {
		if _, ok := capacities[rows]; !ok {
			return "", fmt.Errorf("no receipt-capacity samples for %s rows", phase5RowsLabel(rows))
		}
	}
	if capacities[10_000] >= capacities[100_000] || capacities[100_000] >= capacities[1_000_000] {
		return "", errors.New("receipt capacity does not increase across the archived row sizes")
	}
	return fmt.Sprintf("The receipt slice is retained in `ApplyReport`, and its measured backing capacity increases with the number of batches: %.3f MiB at 10k, %.3f MiB at 100k, and %.3f MiB at 1M.",
		float64(capacities[10_000])/1048576, float64(capacities[100_000])/1048576, float64(capacities[1_000_000])/1048576), nil
}

func phase5SummaryResultRows(samples []phase5Sample) []string {
	groups := phase5GroupByRowsWorkers(samples)
	keys := phase5SortedGroupKeys(groups)
	var rows []string
	for _, key := range keys {
		group := groups[key]
		workflow, apply, rps := make([]int64, 0, len(group)), make([]int64, 0, len(group)), make([]float64, 0, len(group))
		for _, sample := range group {
			workflow = append(workflow, sample.WorkflowNS)
			apply = append(apply, sample.ApplyNS)
			rps = append(rps, sample.RowsPerSecond)
		}
		workflowSummary := fmt.Sprintf("%s (%s–%s)", phase5CommaInt(phase5RoundMillis(phase5MedianInt64(workflow))),
			phase5CommaInt(phase5RoundMillis(phase5MinInt64(workflow))), phase5CommaInt(phase5RoundMillis(phase5MaxInt64(workflow))))
		if len(group) == 1 {
			workflowSummary = phase5CommaInt(phase5RoundMillis(phase5MedianInt64(workflow))) + " (single sample)"
		}
		row := fmt.Sprintf("| %s | %d | %d | %s | %s | %s | %.1f MiB | %.3f MiB | %s |",
			phase5RowsLabel(key[0]), key[1], len(group), workflowSummary,
			phase5CommaInt(phase5RoundMillis(phase5MedianInt64(apply))), phase5CommaInt(int64(math.Round(phase5MedianFloat64(rps)))),
			float64(phase5MaxInt64(phase5Ints64(group, func(s phase5Sample) int64 { return s.PeakRSSKB })))/1024,
			float64(phase5MaxUint64(group, func(s phase5Sample) uint64 { return uint64(s.ReceiptSliceCapacityBytes) }))/1048576,
			phase5CommaInt(int64(group[0].SuccessfulCommitCount)))
		rows = append(rows, row)
	}
	return rows
}

func phase5SummaryStageRows(samples []phase5Sample) []string {
	byRows := make(map[int][]phase5Sample)
	for _, sample := range samples {
		byRows[sample.Rows] = append(byRows[sample.Rows], sample)
	}
	rows := make([]int, 0, len(byRows))
	for n := range byRows {
		rows = append(rows, n)
	}
	sort.Ints(rows)
	var out []string
	for _, n := range rows {
		group := byRows[n]
		median := func(value func(phase5Sample) int64) float64 {
			values := make([]int64, len(group))
			for i, sample := range group {
				values[i] = value(sample)
			}
			return float64(phase5MedianInt64(values)) / float64(time.Millisecond)
		}
		out = append(out, fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |",
			phase5RowsLabel(n), phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.FixtureNS })), phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.ScanNS })),
			phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.PlanNS })), phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.PreviewNS })),
			phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.ApproveNS })), phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.ApplyNS })),
			phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.ValidateNS })), phase5CommaFloatOne(median(func(s phase5Sample) int64 { return s.VerificationNS }))))
	}
	return out
}

func phase5SummaryCPURows(samples []phase5Sample) []string {
	byRows := make(map[int][]phase5Sample)
	for _, sample := range samples {
		byRows[sample.Rows] = append(byRows[sample.Rows], sample)
	}
	rows := make([]int, 0, len(byRows))
	for n := range byRows {
		rows = append(rows, n)
	}
	sort.Ints(rows)
	var out []string
	for _, n := range rows {
		group := byRows[n]
		user, system, total := make([]int64, 0, len(group)), make([]int64, 0, len(group)), make([]int64, 0, len(group))
		for _, sample := range group {
			user = append(user, sample.CPUUserNS)
			system = append(system, sample.CPUSystemNS)
			total = append(total, sample.CPUUserNS+sample.CPUSystemNS)
		}
		out = append(out, fmt.Sprintf("| %s | %.2f s | %.2f s | %.2f s (%.2f–%.2f) |", phase5RowsLabel(n), float64(phase5MedianInt64(user))/1e9,
			float64(phase5MedianInt64(system))/1e9, float64(phase5MedianInt64(total))/1e9, float64(phase5MinInt64(total))/1e9, float64(phase5MaxInt64(total))/1e9))
	}
	return out
}

func phase5ResourceSummary(samples []phase5Sample) (int64, uint64, uint64, int, int) {
	var rss, alloc, inuse uint64
	goroutines, fds := 0, 0
	for _, sample := range samples {
		if uint64(sample.PeakRSSKB) > rss {
			rss = uint64(sample.PeakRSSKB)
		}
		if sample.PeakHeapAllocBytes > alloc {
			alloc = sample.PeakHeapAllocBytes
		}
		if sample.PeakHeapInuseBytes > inuse {
			inuse = sample.PeakHeapInuseBytes
		}
		if sample.PeakGoroutines > goroutines {
			goroutines = sample.PeakGoroutines
		}
		if sample.PeakFDs > fds {
			fds = sample.PeakFDs
		}
	}
	return int64(rss), alloc, inuse, goroutines, fds
}

func phase5GroupByRowsWorkers(samples []phase5Sample) map[[2]int][]phase5Sample {
	groups := make(map[[2]int][]phase5Sample)
	for _, sample := range samples {
		key := [2]int{sample.Rows, sample.Workers}
		groups[key] = append(groups[key], sample)
	}
	return groups
}

func phase5SortedGroupKeys(groups map[[2]int][]phase5Sample) [][2]int {
	keys := make([][2]int, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] == keys[j][0] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})
	return keys
}

func phase5RowsLabel(rows int) string {
	if rows >= 1_000_000 && rows%1_000_000 == 0 {
		return fmt.Sprintf("%dM", rows/1_000_000)
	}
	if rows >= 1_000 && rows%1_000 == 0 {
		return fmt.Sprintf("%dk", rows/1_000)
	}
	return strconv.Itoa(rows)
}

func phase5RoundMillis(nanoseconds int64) int64 {
	return int64(math.Round(float64(nanoseconds) / float64(time.Millisecond)))
}

func phase5CommaInt(value int64) string {
	text := strconv.FormatInt(value, 10)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}

func phase5CommaFloatOne(value float64) string {
	formatted := fmt.Sprintf("%.1f", value)
	parts := strings.SplitN(formatted, ".", 2)
	integer, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || len(parts) != 2 {
		return formatted
	}
	return phase5CommaInt(integer) + "." + parts[1]
}

func phase5SampleEqual(left, right phase5Sample) bool {
	return reflect.DeepEqual(left, right)
}

func clonePhase5CSVRecords(records [][]string) [][]string {
	out := make([][]string, len(records))
	for i, record := range records {
		out[i] = append([]string(nil), record...)
	}
	return out
}
