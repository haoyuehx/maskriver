package runner

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
		if raw.TrialID != sample.TrialID || raw.WorkflowNS != sample.WorkflowNS || raw.ProcessWallNS != sample.ProcessWallNS {
			t.Fatalf("raw and aggregate samples disagree for %s", sample.TrialID)
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
	if matrixArtifact.Completed != len(samples) || len(matrixArtifact.Cases) != len(matrix) {
		t.Fatalf("matrix metadata says completed=%d cases=%d; samples=%d expected_cases=%d", matrixArtifact.Completed, len(matrixArtifact.Cases), len(samples), len(matrix))
	}
	if err := validatePhase5Matrix(samples, matrixArtifact.Cases); err != nil {
		t.Fatalf("matrix metadata disagrees with samples: %v", err)
	}

	var resourceCount int
	for _, sample := range samples {
		rows, err := readPhase5Resources(filepath.Join(dir, "raw", sample.TrialID, "resource-samples.csv"))
		if err != nil {
			t.Fatalf("resource samples %s: %v", sample.TrialID, err)
		}
		if len(rows) != sample.ResourceSampleCount {
			t.Fatalf("resource sample count mismatch for %s: got %d want %d", sample.TrialID, len(rows), sample.ResourceSampleCount)
		}
		resourceCount += len(rows)
	}
	if rows, err := countCSVRows(filepath.Join(dir, "resource-samples.csv")); err != nil || rows != resourceCount {
		t.Fatalf("aggregate resource sample count mismatch: got %d want %d (%v)", rows, resourceCount, err)
	}
	if rows, err := countCSVRows(filepath.Join(dir, "rollup.csv")); err != nil || rows != 12 {
		t.Fatalf("rollup has %d data rows, want 12 (%v)", rows, err)
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
	if err := validatePhase5Matrix(lines, matrix); err != nil {
		t.Fatalf("NDJSON sample set invalid: %v", err)
	}
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

func countCSVRows(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	if _, err := reader.Read(); err != nil {
		return 0, err
	}
	count := 0
	for {
		_, err := reader.Read()
		if err == nil {
			count++
			continue
		}
		if errors.Is(err, io.EOF) {
			return count, nil
		}
		return count, err
	}
}
