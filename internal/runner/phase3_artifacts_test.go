package runner

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3ArtifactConsistency(t *testing.T) {
	dir := os.Getenv("MASK_RIVER_PHASE3_DIR")
	if dir == "" {
		t.Skip("set MASK_RIVER_PHASE3_DIR to verify phase-3 artifacts")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "samples.json"))
	if err != nil {
		t.Fatal(err)
	}
	var samples []phase3Trial
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatal(err)
	}
	matrixSize := len(phase3BatchSizes(samples))
	reps := len(samples) / matrixSize
	if err := validatePhase3Trials(samples, reps); err != nil {
		t.Fatalf("invalid raw phase-3 samples: %v", err)
	}
	environment, err := os.ReadFile(filepath.Join(dir, "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase3Environment(environment, samples, reps); err != nil {
		t.Fatalf("invalid measurement-time environment metadata: %v", err)
	}
	regen := t.TempDir()
	writePhase3Artifacts(t, regen, samples, reps)
	// Environment is measurement-time provenance, not a derived result. It must
	// remain intact and valid, but comparing it to the verifier host would make
	// historical evidence fail whenever HEAD or host metadata changes.
	summaryName := "summary.md"
	if matrixSize == 4 {
		summaryName = "diagnostic-summary.md"
	}
	for _, name := range []string{"samples.json", "apply-transactions.csv", "apply-batches.csv", "results.csv", "results.json", "stats.csv", summaryName, "resource-metrics.json", "throughput.svg"} {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read committed %s: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(regen, name))
		if err != nil {
			t.Fatalf("read regenerated %s: %v", name, err)
		}
		if string(want) != string(got) {
			t.Errorf("%s is not reproducible from samples.json", name)
		}
	}
}

func validatePhase3Environment(raw []byte, samples []phase3Trial, repetitions int) error {
	var env phase3EnvironmentMetadata
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	sha, err := hex.DecodeString(env.HeadSHA)
	if err != nil || len(sha) != 20 {
		return fmt.Errorf("invalid measurement code SHA")
	}
	if env.GOOS == "" || env.GOARCH == "" || env.Go == "" || env.CPU == "" || env.Kernel == "" || env.MemoryTotal == "" || env.MemoryScope == "" || env.Driver == "" || env.Command == "" {
		return fmt.Errorf("required host/provenance field missing")
	}
	if env.Repetitions != repetitions || env.Samples != len(samples) || env.Seed != "0x132026" {
		return fmt.Errorf("environment matrix metadata disagrees with raw samples")
	}
	for _, sample := range samples {
		if sample.JournalMode != env.Journal || sample.Synchronous != env.Synchronous {
			return fmt.Errorf("environment SQLite settings disagree with raw samples")
		}
	}
	return nil
}
