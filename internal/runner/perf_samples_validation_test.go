package runner

import (
	"os"
	"strings"
	"testing"
)

func validPerfSamples() []perfSample {
	var samples []perfSample
	for _, cfg := range perfMatrix() {
		samples = append(samples, perfSample{
			Rows: cfg.rows, Shape: shapeOf(cfg.long), Workers: cfg.workers, Iteration: 1,
			TotalNS: 1_000_000, ScanNS: 200_000, PlanNS: 200_000, PreviewNS: 200_000, ApproveNS: 200_000, ApplyNS: 200_000,
			RowsDone: int64(cfg.rows), FixtureNS: 500,
			GoroutinesBefore: 1, GoroutinesAfter: 1,
			RSSBeforeKB: -1, RSSAfterKB: -1, CPUUserNS: -1, CPUSystemNS: -1,
			ReaderOpen: -1, WriterOpen: -1, ReaderInUse: -1, WriterInUse: -1,
		})
	}
	return samples
}

func TestValidateSamplesRejectsPhysicalAndMatrixViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]perfSample) []perfSample
		want   string
	}{
		{"rows masked zero", func(s []perfSample) []perfSample { s[0].RowsDone = 0; return s }, "rows_masked"},
		{"stage sum 3 percent", func(s []perfSample) []perfSample {
			each := s[0].TotalNS * 6 / 1000
			s[0].ScanNS, s[0].PlanNS, s[0].PreviewNS, s[0].ApproveNS, s[0].ApplyNS = each, each, each, each, each
			return s
		}, "stage sum is"},
		{"illegal workers", func(s []perfSample) []perfSample { s[0].Workers = 3; return s }, "illegal configuration"},
		{"missing sample", func(s []perfSample) []perfSample { return s[1:] }, "configuration"},
		{"duplicate iteration", func(s []perfSample) []perfSample {
			s[1].Rows, s[1].Shape, s[1].Workers = s[0].Rows, s[0].Shape, s[0].Workers
			return s
		}, "duplicate sample"},
		{"zero total", func(s []perfSample) []perfSample { s[0].TotalNS = 0; return s }, "non-positive"},
		{"unverified sentinel only", func(s []perfSample) []perfSample { s[0].RSSAfterKB = -2; return s }, "UNVERIFIED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			samples := tt.mutate(validPerfSamples())
			err := validateSamples(samples)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateSamples error=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateSamplesAcceptsCompleteMatrixAndUnverifiedResources(t *testing.T) {
	if err := validateSamples(validPerfSamples()); err != nil {
		t.Fatalf("validateSamples: %v", err)
	}
}

func TestBenchIterationResourcesAreRemovedAfterClose(t *testing.T) {
	root := t.TempDir()
	env := benchEnvIn(t, benchConfig{rows: 8, workers: 1}, root)
	if err := env.close(); err != nil {
		t.Fatalf("release iteration resources: %v", err)
	}
	reader, writer, ok := env.engine.PoolStats()
	if !ok || reader.OpenConnections != 0 || writer.OpenConnections != 0 {
		t.Fatalf("connection pool not released after close: ok=%v reader=%+v writer=%+v", ok, reader, writer)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read bounded root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("iteration resources remain after close: %v", entries)
	}
}
