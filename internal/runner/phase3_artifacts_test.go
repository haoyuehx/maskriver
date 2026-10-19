package runner

import (
	"encoding/json"
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
	reps := len(samples) / 3
	if err := validatePhase3Trials(samples, reps); err != nil {
		t.Fatalf("invalid raw phase-3 samples: %v", err)
	}
	regen := t.TempDir()
	writePhase3Artifacts(t, regen, samples, reps)
	for _, name := range []string{"samples.json", "apply-transactions.csv", "apply-batches.csv", "results.csv", "results.json", "stats.csv", "summary.md", "environment.json", "resource-metrics.json", "throughput.svg"} {
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
