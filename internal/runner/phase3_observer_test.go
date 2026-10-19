package runner

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const observerRepeats = 10

type observerSample struct {
	Order        int   `json:"order"`
	Trial        int   `json:"trial"`
	BatchSize    int   `json:"batch_size"`
	Instrumented bool  `json:"instrumented"`
	ApplyNS      int64 `json:"apply_ns"`
	Rows         int64 `json:"rows"`
	Receipts     int   `json:"receipts"`
	Complete     bool  `json:"complete"`
	Validate     bool  `json:"independent_validate_passed"`
}

func TestPhase3ObserverOverhead(t *testing.T) {
	outDir := os.Getenv("MASK_RIVER_PHASE3_OVERHEAD_DIR")
	if outDir == "" {
		t.Skip("set MASK_RIVER_PHASE3_OVERHEAD_DIR to measure observer overhead")
	}
	type pair struct{ batch, trial int }
	pairs := make([]pair, 0, 3*observerRepeats)
	for _, batch := range []int{16, 64, 256} {
		for trial := 1; trial <= observerRepeats; trial++ {
			pairs = append(pairs, pair{batch, trial})
		}
	}
	rng := rand.New(rand.NewSource(0x13_2027))
	rng.Shuffle(len(pairs), func(i, j int) { pairs[i], pairs[j] = pairs[j], pairs[i] })
	samples := make([]observerSample, 0, len(pairs)*2)
	order := 0
	for _, p := range pairs {
		instrumentedFirst := rng.Intn(2) == 0
		plainRows, spyRows := map[int64]string(nil), map[int64]string(nil)
		for _, instrumented := range []bool{instrumentedFirst, !instrumentedFirst} {
			order++
			sample, rows := runObserverSample(t, order, p.trial, p.batch, instrumented)
			samples = append(samples, sample)
			if instrumented {
				spyRows = rows
			} else {
				plainRows = rows
			}
		}
		if !sameRows(plainRows, spyRows) {
			t.Fatalf("batch=%d trial=%d SQLSpy changed per-key output", p.batch, p.trial)
		}
	}
	writeObserverArtifacts(t, outDir, samples)
	t.Logf("wrote %d paired observations (plain vs SQLSpy) for 256 rows and batch 16/64/256", len(samples))
}

func runObserverSample(t *testing.T, order, trial, batch int, instrumented bool) (observerSample, map[int64]string) {
	t.Helper()
	var sample observerSample
	var rows map[int64]string
	name := fmt.Sprintf("order-%02d/batch-%d/trial-%02d/instrumented-%t", order, batch, trial, instrumented)
	if !t.Run(name, func(t *testing.T) {
		fixture := newSignedEngine(t, syntheticEmails(256), 4)
		plan := fixture.prepare(t)
		if instrumented {
			spy := db.NewSQLSpy()
			fixture.engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
				writer, err := spy.OpenWriter(ctx, opts)
				if err != nil {
					return nil, err
				}
				return &spyBatchWriter{Writer: writer, sqlSpy: spy}, nil
			}
		}
		started := time.Now()
		report, err := fixture.engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: batch})
		elapsed := time.Since(started).Nanoseconds()
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		var committed int64
		for _, receipt := range report.Committed {
			committed += receipt.Rows
		}
		wantReceipts := (256 + batch - 1) / batch
		if !report.Complete || committed != 256 || len(report.Committed) != wantReceipts {
			t.Fatalf("Apply complete=%v rows=%d receipts=%d, want true/256/%d", report.Complete, committed, len(report.Committed), wantReceipts)
		}
		validation, err := fixture.engine.Validate(context.Background(), contracts.ValidateRequest{Plan: plan})
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
		passed := validation.Coverage.Complete && validation.Passed(true)
		if !passed {
			t.Fatalf("independent Validate failed: %+v", validation.Issues)
		}
		rows = targetRows(t, fixture.cfg.Target)
		if len(rows) != 256 {
			t.Fatalf("target row count=%d, want 256", len(rows))
		}
		sample = observerSample{Order: order, Trial: trial, BatchSize: batch, Instrumented: instrumented, ApplyNS: elapsed, Rows: committed, Receipts: len(report.Committed), Complete: report.Complete, Validate: passed}
	}) {
		t.Fatalf("observation subtest failed: %s", name)
	}
	return sample, rows
}

func validateObserverSamples(samples []observerSample) error {
	if len(samples) != observerRepeats*3*2 {
		return fmt.Errorf("sample count=%d, want %d", len(samples), observerRepeats*3*2)
	}
	type key struct{ batch, trial int }
	seen := map[key]map[bool]bool{}
	orders := map[int]bool{}
	for _, s := range samples {
		if (s.BatchSize != 16 && s.BatchSize != 64 && s.BatchSize != 256) || s.Trial < 1 || s.Trial > observerRepeats || s.Order < 1 || s.Order > len(samples) || orders[s.Order] || s.ApplyNS <= 0 || s.Rows != 256 || s.Receipts != (256+s.BatchSize-1)/s.BatchSize || !s.Complete || !s.Validate {
			return fmt.Errorf("invalid observer sample order=%d batch=%d trial=%d", s.Order, s.BatchSize, s.Trial)
		}
		orders[s.Order] = true
		k := key{s.BatchSize, s.Trial}
		if seen[k] == nil {
			seen[k] = map[bool]bool{}
		}
		if seen[k][s.Instrumented] {
			return fmt.Errorf("duplicate observer mode batch=%d trial=%d instrumented=%t", s.BatchSize, s.Trial, s.Instrumented)
		}
		seen[k][s.Instrumented] = true
	}
	for _, batch := range []int{16, 64, 256} {
		for trial := 1; trial <= observerRepeats; trial++ {
			if len(seen[key{batch, trial}]) != 2 {
				return fmt.Errorf("missing paired observer sample batch=%d trial=%d", batch, trial)
			}
		}
	}
	return nil
}

func observerCSV(samples []observerSample) string {
	ordered := append([]observerSample(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Order < ordered[j].Order })
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"order", "trial", "batch_size", "instrumented", "apply_ns", "rows", "receipts", "complete", "independent_validate_passed"})
	for _, s := range ordered {
		_ = w.Write([]string{strconv.Itoa(s.Order), strconv.Itoa(s.Trial), strconv.Itoa(s.BatchSize), strconv.FormatBool(s.Instrumented), strconv.FormatInt(s.ApplyNS, 10), strconv.FormatInt(s.Rows, 10), strconv.Itoa(s.Receipts), strconv.FormatBool(s.Complete), strconv.FormatBool(s.Validate)})
	}
	w.Flush()
	return b.String()
}

func observerSummary(samples []observerSample) string {
	var b strings.Builder
	b.WriteString("# SQLSpy observer overhead (paired, 256 rows; descriptive only)\n\n")
	b.WriteString("Each pair uses independent fresh signed SQLite fixtures, randomized mode order, equal per-key outputs and independent Validate. `plain` uses the production SQL path; `instrumented` adds SQLSpy plus diagnostic writer counters. Apply wall time includes all observer work. This estimates total observer perturbation for these test fixtures; it does not imply production throughput or statistical significance.\n\n")
	b.WriteString("| Batch | pairs | plain Apply median (ms) | instrumented Apply median (ms) | paired delta median (%) | paired delta min–max (%) |\n|---:|---:|---:|---:|---:|---:|\n")
	for _, batch := range []int{16, 64, 256} {
		plain, spy, delta := []float64{}, []float64{}, []float64{}
		for trial := 1; trial <= observerRepeats; trial++ {
			var p, q int64
			for _, s := range samples {
				if s.BatchSize == batch && s.Trial == trial {
					if s.Instrumented {
						q = s.ApplyNS
					} else {
						p = s.ApplyNS
					}
				}
			}
			plain = append(plain, float64(p))
			spy = append(spy, float64(q))
			delta = append(delta, 100*(float64(q)/float64(p)-1))
		}
		ps, qs, ds := statsOf(plain), statsOf(spy), statsOf(delta)
		fmt.Fprintf(&b, "| %d | %d | %.2f | %.2f | %.2f | %.2f–%.2f |\n", batch, observerRepeats, ps.Median/1e6, qs.Median/1e6, ds.Median, ds.Min, ds.Max)
	}
	return b.String()
}

func writeObserverArtifacts(t *testing.T, dir string, samples []observerSample) {
	t.Helper()
	if err := validateObserverSamples(samples); err != nil {
		t.Fatalf("invalid observer samples: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "observer-samples.json"), string(data)+"\n")
	writeFile(t, filepath.Join(dir, "observer-results.csv"), observerCSV(samples))
	writeFile(t, filepath.Join(dir, "observer-summary.md"), observerSummary(samples))
}

func TestPhase3ObserverOverheadArtifactConsistency(t *testing.T) {
	dir := os.Getenv("MASK_RIVER_PHASE3_OVERHEAD_DIR")
	if dir == "" {
		t.Skip("set MASK_RIVER_PHASE3_OVERHEAD_DIR to verify observer artifacts")
	}
	data, err := os.ReadFile(filepath.Join(dir, "observer-samples.json"))
	if err != nil {
		t.Fatal(err)
	}
	var samples []observerSample
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	if err := validateObserverSamples(samples); err != nil {
		t.Fatalf("invalid raw observer samples: %v", err)
	}
	regen := t.TempDir()
	writeObserverArtifacts(t, regen, samples)
	for _, name := range []string{"observer-samples.json", "observer-results.csv", "observer-summary.md"} {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(regen, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("%s is not reproducible from observer-samples.json", name)
		}
	}
}
