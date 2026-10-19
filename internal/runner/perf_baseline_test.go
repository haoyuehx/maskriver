package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This file produces the Issue #13 baseline from one raw sample series. Every
// artifact is derived here, so a number can never be hand-copied from a different
// run, and the summary is regenerated rather than edited.
//
// The test only runs when MASK_RIVER_PERF_DIR is set, because a full matrix is a
// measurement job, not something a normal `go test` should do.

// perfSample is one iteration of one configuration. Stage durations and the total
// come from the same iteration, so their ratio is meaningful; they are never mixed
// with an average from a different measurement.
type perfSample struct {
	Rows             int    `json:"rows"`
	Shape            string `json:"shape"`
	Workers          int    `json:"workers"`
	Iteration        int    `json:"iteration"`
	TotalNS          int64  `json:"total_ns"`
	ScanNS           int64  `json:"scan_ns"`
	PlanNS           int64  `json:"plan_ns"`
	PreviewNS        int64  `json:"preview_ns"`
	ApproveNS        int64  `json:"approve_ns"`
	ApplyNS          int64  `json:"apply_ns"`
	RowsDone         int64  `json:"rows_masked"`
	FixtureNS        int64  `json:"fixture_ns"`
	GoroutinesBefore int    `json:"goroutines_before"`
	GoroutinesAfter  int    `json:"goroutines_after"`
	RSSBeforeKB      int64  `json:"rss_before_kb"`
	RSSAfterKB       int64  `json:"rss_after_kb"`
	CPUUserNS        int64  `json:"cpu_user_ns"`
	CPUSystemNS      int64  `json:"cpu_system_ns"`
	ReaderOpen       int    `json:"reader_open_conns_after"`
	WriterOpen       int    `json:"writer_open_conns_after"`
	ReaderInUse      int    `json:"reader_in_use_after"`
	WriterInUse      int    `json:"writer_in_use_after"`
}

type perfConfig struct {
	rows    int
	long    bool
	workers int
}

func perfMatrix() []perfConfig {
	var out []perfConfig
	for _, rows := range []int{128, 512, 2048} {
		for _, long := range []bool{false, true} {
			for _, workers := range []int{1, 4} {
				out = append(out, perfConfig{rows: rows, long: long, workers: workers})
			}
		}
	}
	return out
}

func shapeOf(long bool) string {
	if long {
		return "long"
	}
	return "short"
}

// rssKB reads VmRSS from /proc. Non-Linux platforms report -1, which the artifacts
// record as UNVERIFIED instead of a fabricated zero.
func rssKB() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if value, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return value
				}
			}
		}
	}
	return -1
}

// measureConfig runs iterations of one configuration. Each iteration builds a fresh
// environment outside the measured region and closes its connections before the
// next one starts, so the target database is never overwritten underneath an open
// handle and no state carries over between iterations.
func measureConfig(t *testing.T, cfg perfConfig, iterations int) []perfSample {
	t.Helper()
	samples := make([]perfSample, 0, iterations)
	for i := 0; i < iterations; i++ {
		fixtureStart := time.Now()
		env := benchEnvFor(t, benchConfig{rows: cfg.rows, long: cfg.long, workers: cfg.workers})
		fixtureNS := time.Since(fixtureStart).Nanoseconds()
		goroutinesBefore := runtime.NumGoroutine()
		rssBefore := rssKB()
		userBefore, systemBefore, cpuOK := cpuNS()

		start := time.Now()
		stages, rows := env.run(t)
		totalNS := time.Since(start).Nanoseconds()

		userAfter, systemAfter, cpuAfterOK := cpuNS()
		reader, writer, poolOK := env.engine.PoolStats()
		sample := perfSample{
			Rows: cfg.rows, Shape: shapeOf(cfg.long), Workers: cfg.workers, Iteration: i + 1,
			TotalNS: totalNS, RowsDone: rows, FixtureNS: fixtureNS,
			CPUUserNS: -1, CPUSystemNS: -1,
			ScanNS:           stages["scan"].Nanoseconds(),
			PlanNS:           stages["plan"].Nanoseconds(),
			PreviewNS:        stages["preview"].Nanoseconds(),
			ApproveNS:        stages["approve"].Nanoseconds(),
			ApplyNS:          stages["apply"].Nanoseconds(),
			GoroutinesBefore: goroutinesBefore, GoroutinesAfter: runtime.NumGoroutine(),
			RSSBeforeKB: rssBefore, RSSAfterKB: rssKB(),
			ReaderOpen: -1, WriterOpen: -1, ReaderInUse: -1, WriterInUse: -1,
		}
		if cpuOK && cpuAfterOK {
			sample.CPUUserNS = userAfter - userBefore
			sample.CPUSystemNS = systemAfter - systemBefore
		}
		if poolOK {
			sample.ReaderOpen = reader.OpenConnections
			sample.WriterOpen = writer.OpenConnections
			sample.ReaderInUse = reader.InUse
			sample.WriterInUse = writer.InUse
		}
		if err := env.close(); err != nil {
			t.Errorf("close measured engine and remove fixture: %v", err)
		}
		samples = append(samples, sample)
	}
	return samples
}

// validateSamples checks physical and matrix invariants for both newly measured
// data and offline artifact verification. The 80-120% stage/total tolerance is
// intentionally broad: TotalNS wraps env.run while stages use their own clocks;
// the margin permits timer overhead but rejects fixture contamination and
// truncated/missing work.
func validateSamples(samples []perfSample) error {
	if len(samples) == 0 {
		return fmt.Errorf("no samples")
	}
	configs := make(map[string]perfConfig)
	for _, cfg := range perfMatrix() {
		configs[fmt.Sprintf("%d/%s/w%d", cfg.rows, shapeOf(cfg.long), cfg.workers)] = cfg
	}
	counts := make(map[string]int)
	seen := make(map[string]bool)
	iterations := 0
	for _, s := range samples {
		key := fmt.Sprintf("%d/%s/w%d", s.Rows, s.Shape, s.Workers)
		if _, ok := configs[key]; !ok {
			return fmt.Errorf("illegal configuration %s", key)
		}
		if s.Iteration < 1 {
			return fmt.Errorf("%s has invalid iteration %d", key, s.Iteration)
		}
		id := fmt.Sprintf("%s/iteration=%d", key, s.Iteration)
		if seen[id] {
			return fmt.Errorf("duplicate sample %s", id)
		}
		seen[id] = true
		counts[key]++
		if s.Iteration > iterations {
			iterations = s.Iteration
		}
		if s.RowsDone != int64(s.Rows) {
			return fmt.Errorf("%s iteration %d rows_masked=%d, want %d", key, s.Iteration, s.RowsDone, s.Rows)
		}
		if s.TotalNS <= 0 || s.FixtureNS <= 0 {
			return fmt.Errorf("%s iteration %d has non-positive total/fixture duration", key, s.Iteration)
		}
		stages := []int64{s.ScanNS, s.PlanNS, s.PreviewNS, s.ApproveNS, s.ApplyNS}
		for i, ns := range stages {
			if ns <= 0 {
				return fmt.Errorf("%s iteration %d stage %d has non-positive duration %d", key, s.Iteration, i, ns)
			}
		}
		ratio := 100 * float64(s.stageSumNS()) / float64(s.TotalNS)
		if ratio < 80 || ratio > 120 {
			return fmt.Errorf("%s iteration %d stage sum is %.1f%% of total, expected 80-120%%", key, s.Iteration, ratio)
		}
		if s.GoroutinesBefore < 0 || s.GoroutinesAfter < 0 || !validMetric(s.RSSBeforeKB) || !validMetric(s.RSSAfterKB) ||
			!validMetric(s.CPUUserNS) || !validMetric(s.CPUSystemNS) || !validMetric(int64(s.ReaderOpen)) ||
			!validMetric(int64(s.WriterOpen)) || !validMetric(int64(s.ReaderInUse)) || !validMetric(int64(s.WriterInUse)) {
			return fmt.Errorf("%s iteration %d has invalid resource metric; use -1 for UNVERIFIED", key, s.Iteration)
		}
		if (s.CPUUserNS == -1) != (s.CPUSystemNS == -1) {
			return fmt.Errorf("%s iteration %d CPU metrics must both be measured or both -1 (UNVERIFIED)", key, s.Iteration)
		}
	}
	if iterations == 0 {
		return fmt.Errorf("no valid iteration count")
	}
	for key := range configs {
		if counts[key] != iterations {
			return fmt.Errorf("configuration %s has %d samples; matrix requires %d iterations", key, counts[key], iterations)
		}
		for i := 1; i <= iterations; i++ {
			if !seen[fmt.Sprintf("%s/iteration=%d", key, i)] {
				return fmt.Errorf("configuration %s missing iteration %d", key, i)
			}
		}
	}
	if len(samples) != len(configs)*iterations {
		return fmt.Errorf("sample count %d does not match %d configurations × %d iterations", len(samples), len(configs), iterations)
	}
	return nil
}

// -1 is the explicit UNVERIFIED sentinel; zero is a real value only for counts.
func validMetric(value int64) bool { return value == -1 || value >= 0 }

// stageSum is the sum of the stages measured inside the same iteration. Comparing
// it with TotalNS is the pollution check: if fixture work leaked into the measured
// region the two would diverge widely.
func (s perfSample) stageSumNS() int64 {
	return s.ScanNS + s.PlanNS + s.PreviewNS + s.ApproveNS + s.ApplyNS
}

type stageStats struct {
	Count     int
	Mean      float64
	Median    float64
	Min       float64
	Max       float64
	StdDevPct float64
}

func statsOf(values []float64) stageStats {
	if len(values) == 0 {
		return stageStats{}
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	stats := stageStats{Count: len(ordered), Min: ordered[0], Max: ordered[len(ordered)-1]}
	var sum float64
	for _, value := range ordered {
		sum += value
	}
	stats.Mean = sum / float64(len(ordered))
	if len(ordered)%2 == 1 {
		stats.Median = ordered[len(ordered)/2]
	} else {
		stats.Median = (ordered[len(ordered)/2-1] + ordered[len(ordered)/2]) / 2
	}
	var variance float64
	for _, value := range ordered {
		delta := value - stats.Mean
		variance += delta * delta
	}
	if len(ordered) > 1 && stats.Mean != 0 {
		stats.StdDevPct = 100 * sqrt(variance/float64(len(ordered)-1)) / stats.Mean
	}
	return stats
}

func sqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}
	guess := value
	for i := 0; i < 40; i++ {
		guess = (guess + value/guess) / 2
	}
	return guess
}

func perConfig(samples []perfSample) map[string][]perfSample {
	groups := map[string][]perfSample{}
	for _, sample := range samples {
		key := fmt.Sprintf("%d/%s/w%d", sample.Rows, sample.Shape, sample.Workers)
		groups[key] = append(groups[key], sample)
	}
	return groups
}

// writeBaselineArtifacts derives every artifact from the samples and returns the
// rendered summary, which the consistency test regenerates and compares.
func writeBaselineArtifacts(t *testing.T, dir string, samples []perfSample) string {
	t.Helper()
	if err := validateSamples(samples); err != nil {
		t.Fatalf("invalid performance samples: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		t.Fatalf("marshal samples: %v", err)
	}
	writeFile(t, filepath.Join(dir, "samples.json"), string(raw)+"\n")

	groups := perConfig(samples)
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := groups[keys[i]][0], groups[keys[j]][0]
		if left.Rows != right.Rows {
			return left.Rows < right.Rows
		}
		if left.Shape != right.Shape {
			return left.Shape < right.Shape
		}
		return left.Workers < right.Workers
	})

	var csv strings.Builder
	csv.WriteString("rows,shape,workers,iterations,total_ns_mean,total_ns_median,rows_per_s_mean,stage_sum_ratio_pct,apply_pct_of_total,scan_ns_median,plan_ns_median,preview_ns_median,approve_ns_median,apply_ns_median,fixture_ns_median,rss_after_kb_median,cpu_user_ns_median,cpu_system_ns_median,stddev_pct\n")
	var summary strings.Builder
	summary.WriteString("| rows | shape | workers | iterations | total mean | total median | rows/s (mean) | stage/total | apply share | apply median |\n")
	summary.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	var resources strings.Builder
	resources.WriteString("| rows | shape | workers | fixture median | RSS after median | CPU user median | CPU system median | reader open | writer open |\n")
	resources.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, key := range keys {
		group := groups[key]
		var totals, stageRatio, applyPct, rowsPerSec, fixtures, rss, cpuUser, cpuSystem []float64
		var scan, plan, preview, approve, apply []float64
		for _, sample := range group {
			totals = append(totals, float64(sample.TotalNS))
			stageRatio = append(stageRatio, 100*float64(sample.stageSumNS())/float64(sample.TotalNS))
			applyPct = append(applyPct, 100*float64(sample.ApplyNS)/float64(sample.TotalNS))
			rowsPerSec = append(rowsPerSec, float64(sample.RowsDone)/(float64(sample.TotalNS)/1e9))
			fixtures = append(fixtures, float64(sample.FixtureNS))
			scan = append(scan, float64(sample.ScanNS))
			plan = append(plan, float64(sample.PlanNS))
			preview = append(preview, float64(sample.PreviewNS))
			approve = append(approve, float64(sample.ApproveNS))
			apply = append(apply, float64(sample.ApplyNS))
			if sample.RSSAfterKB >= 0 {
				rss = append(rss, float64(sample.RSSAfterKB))
			}
			if sample.CPUUserNS > 0 {
				cpuUser = append(cpuUser, float64(sample.CPUUserNS))
				cpuSystem = append(cpuSystem, float64(sample.CPUSystemNS))
			}
		}
		totalStats := statsOf(totals)
		rowStats := statsOf(rowsPerSec)
		applyStats := statsOf(apply)
		stageStatsValue := statsOf(stageRatio)
		applyShare := statsOf(applyPct)
		fixtureStats := statsOf(fixtures)
		rssStats := statsOf(rss)
		cpuUserStats := statsOf(cpuUser)
		cpuSystemStats := statsOf(cpuSystem)
		fmt.Fprintf(&csv, "%d,%s,%d,%d,%.0f,%.0f,%.2f,%.2f,%.2f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.2f\n",
			group[0].Rows, group[0].Shape, group[0].Workers, len(group),
			totalStats.Mean, totalStats.Median, rowStats.Mean, stageStatsValue.Mean, applyShare.Mean,
			statsOf(scan).Median, statsOf(plan).Median, statsOf(preview).Median,
			statsOf(approve).Median, applyStats.Median, fixtureStats.Median,
			rssStats.Median, cpuUserStats.Median, cpuSystemStats.Median, totalStats.StdDevPct)
		var connRead, connWrite []float64
		for _, sample := range group {
			if sample.ReaderOpen >= 0 {
				connRead = append(connRead, float64(sample.ReaderOpen))
				connWrite = append(connWrite, float64(sample.WriterOpen))
			}
		}
		fmt.Fprintf(&summary, "| %d | %s | %d | %d | %.1f ms | %.1f ms | %.0f | %.0f%% | %.0f%% | %.1f ms |\n",
			group[0].Rows, group[0].Shape, group[0].Workers, len(group),
			totalStats.Mean/1e6, totalStats.Median/1e6, rowStats.Mean,
			stageStatsValue.Mean, applyShare.Mean, applyStats.Median/1e6)
		fmt.Fprintf(&resources, "| %d | %s | %d | %.1f ms | %s | %s | %s | %s | %s |\n",
			group[0].Rows, group[0].Shape, group[0].Workers, fixtureStats.Median/1e6,
			renderResource(rssStats.Median, rssStats.Count, "%.0f KB"), renderResource(cpuUserStats.Median/1e6, cpuUserStats.Count, "%.0f ms"),
			renderResource(cpuSystemStats.Median/1e6, cpuSystemStats.Count, "%.0f ms"),
			renderResource(statsOf(connRead).Median, len(connRead), "%.0f"), renderResource(statsOf(connWrite).Median, len(connWrite), "%.0f"))
	}
	writeFile(t, filepath.Join(dir, "results.csv"), csv.String())
	writeFile(t, filepath.Join(dir, "environment.json"), environmentJSON(samples))

	// The CSV reports min/max/stddev too, so a reader can see spread rather than
	// trusting a single summary figure.
	writeFile(t, filepath.Join(dir, "stats.csv"), statsCSV(keys, groups))

	rendered := renderSummary(keys, groups, len(samples), summary.String(), resources.String())
	writeFile(t, filepath.Join(dir, "summary.md"), rendered)
	return rendered
}

// renderSummary composes the document from the computed table plus environment
// facts. It is regenerated on every measurement run; nothing in it is typed by
// hand.
func renderResource(value float64, count int, format string) string {
	if count == 0 {
		return "UNVERIFIED"
	}
	return fmt.Sprintf(format, value)
}

// statsCSV reports the full spread per configuration, so mean, median, min, max
// and standard deviation are all visible and none of them has to be inferred from
// a rounded table.
func statsCSV(keys []string, groups map[string][]perfSample) string {
	var b strings.Builder
	b.WriteString("rows,shape,workers,metric,count,mean,median,min,max,stddev_pct\n")
	for _, key := range keys {
		group := groups[key]
		metrics := map[string][]float64{}
		for _, sample := range group {
			metrics["total_ns"] = append(metrics["total_ns"], float64(sample.TotalNS))
			metrics["apply_ns"] = append(metrics["apply_ns"], float64(sample.ApplyNS))
			metrics["fixture_ns"] = append(metrics["fixture_ns"], float64(sample.FixtureNS))
			metrics["rows_per_s"] = append(metrics["rows_per_s"], float64(sample.RowsDone)/(float64(sample.TotalNS)/1e9))
			if sample.RSSAfterKB >= 0 {
				metrics["rss_after_kb"] = append(metrics["rss_after_kb"], float64(sample.RSSAfterKB))
			}
			if sample.CPUUserNS > 0 {
				metrics["cpu_user_ns"] = append(metrics["cpu_user_ns"], float64(sample.CPUUserNS))
				metrics["cpu_system_ns"] = append(metrics["cpu_system_ns"], float64(sample.CPUSystemNS))
			}
		}
		names := make([]string, 0, len(metrics))
		for name := range metrics {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stats := statsOf(metrics[name])
			fmt.Fprintf(&b, "%d,%s,%d,%s,%d,%.1f,%.1f,%.1f,%.1f,%.2f\n",
				group[0].Rows, group[0].Shape, group[0].Workers, name,
				stats.Count, stats.Mean, stats.Median, stats.Min, stats.Max, stats.StdDevPct)
		}
	}
	return b.String()
}

// environmentJSON records the machine and command so a run can be reproduced.
func environmentJSON(samples []perfSample) string {
	type environment struct {
		GOOS          string `json:"goos"`
		GOARCH        string `json:"goarch"`
		GoVersion     string `json:"go_version"`
		Driver        string `json:"sqlite_driver"`
		CPU           string `json:"cpu"`
		Cores         int    `json:"cores"`
		Command       string `json:"command"`
		Iterations    int    `json:"iterations"`
		SampleCount   int    `json:"sample_count"`
		TimerScope    string `json:"timer_scope"`
		ResourceScope string `json:"resource_scope"`
	}
	iterations := 0
	for _, sample := range samples {
		if sample.Iteration > iterations {
			iterations = sample.Iteration
		}
	}
	encoded, _ := json.MarshalIndent(environment{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(),
		Driver: "modernc.org/sqlite v1.60.1 (pure Go)", CPU: hostCPU(), Cores: runtime.NumCPU(),
		Command:       "MASK_RIVER_PERF_DIR=<dir> MASK_RIVER_PERF_ITERS=" + strconv.Itoa(iterations) + " go test -run 'TestPerfBaseline' -timeout 50m ./internal/runner/",
		Iterations:    iterations,
		SampleCount:   len(samples),
		TimerScope:    "per iteration: Scan, Plan, Preview, Approve, Apply measured individually; fixture construction and connection close happen outside the measured region",
		ResourceScope: "RSS and CPU are process-wide deltas across one iteration; the reader/writer pool counters are those of the engine's own connections",
	}, "", "  ")
	return string(encoded) + "\n"
}

func hostCPU() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "UNVERIFIED"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "UNVERIFIED"
}

func renderSummary(keys []string, groups map[string][]perfSample, sampleCount int, table, resources string) string {
	var first, last perfSample
	first, last = groups[keys[0]][0], groups[keys[len(keys)-1]][0]
	var b strings.Builder
	fmt.Fprintf(&b, "# Issue #13 真实 SQLite Runner 性能基线\n\n")
	fmt.Fprintf(&b, "本文件由测量程序生成，不是手工填写。原始逐次样本在同目录 `samples.json`，聚合值在 `results.csv`；`TestPerfArtifactsMatchSamples` 会重新计算并比对，手改数字会让测试失败。\n\n")
	fmt.Fprintf(&b, "## 口径\n\n")
	fmt.Fprintf(&b, "- **样本**：每个配置每次迭代是一条 `samples.json` 记录，含工作流总耗时与该次迭代内五个阶段的耗时。\n")
	fmt.Fprintf(&b, "- **total**：单次迭代内 `Scan→Plan→Preview→Approve→Apply` 的墙钟耗时，不含 fixture 构建、连接关闭、target 重建。\n")
	fmt.Fprintf(&b, "- **stage/total**：同一次迭代内阶段之和除以该次总耗时。它是污染检查：fixture 若混入计时区间，该比值会明显偏离 100%%。\n")
	fmt.Fprintf(&b, "- **apply share**：同一次迭代内 `apply` 除以总耗时。\n")
	fmt.Fprintf(&b, "- **rows/s**：单次迭代 `rows_masked / total`，不是用平均值反推。\n")
	fmt.Fprintf(&b, "- **median/mean/min/max/stddev**：全部逐项记录在 `stats.csv`；下表 total 用 mean 与 median，rows/s 与占比用 mean，apply 用 median，均明确标注。\n")
	fmt.Fprintf(&b, "- **fixture**：单独计时，`fixture_ns_median` 列，不计入任何工作流数字。\n\n")
	fmt.Fprintf(&b, "## 环境\n\n")
	fmt.Fprintf(&b, "- 平台：%s/%s，Go %s，SQLite 驱动 modernc.org/sqlite v1.60.1（纯 Go）\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(&b, "- 首配置：%d 行，末配置：%d 行；配置 %d 组，逐次样本 %d 条\n\n", first.Rows, last.Rows, len(keys), sampleCount)
	b.WriteString("## 工作流结果\n\n")
	b.WriteString(table)
	b.WriteString("\n## 资源与 fixture（口径见 environment.json）\n\n")
	b.WriteString("RSS/CPU 是**单次迭代**期间的进程级增量，不含 fixture 构建；`fixture median` 是单独计时的 fixture 成本，**不计入**上面的工作流数字。\n\n")
	b.WriteString(resources)
	b.WriteString("\n")
	b.WriteString("## 未测量 / UNVERIFIED\n\n")
	b.WriteString("- 显示为 UNVERIFIED 的列在本机或本平台无法采集（例如非 Linux 上的 /proc 统计）。\n")
	b.WriteString("- 10k/100k/1M 行、workers 2/8/16 与本轮矩阵之外的平台：**未运行**。\n")
	b.WriteString("- 本文件不声明统计显著性；看 `stats.csv` 的 `stddev_pct` 与 min/max 自行判断。\n")
	return b.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestPerfBaseline measures the matrix and writes the artifacts. It requires
// MASK_RIVER_PERF_DIR; MASK_RIVER_PERF_ITERS sets the iteration count (default 5).
func TestPerfBaseline(t *testing.T) {
	dir := os.Getenv("MASK_RIVER_PERF_DIR")
	if dir == "" {
		t.Skip("set MASK_RIVER_PERF_DIR to run the performance baseline")
	}
	// Regenerating from existing samples is how the committed artifacts are
	// refreshed without re-measuring, and it guarantees they come from the raw
	// samples rather than from a previous report.
	if os.Getenv("MASK_RIVER_PERF_REGEN") != "" {
		raw, err := os.ReadFile(filepath.Join(dir, "samples.json"))
		if err != nil {
			t.Fatalf("read samples: %v", err)
		}
		var samples []perfSample
		if err := json.Unmarshal(raw, &samples); err != nil {
			t.Fatalf("parse samples: %v", err)
		}
		if err := validateSamples(samples); err != nil {
			t.Fatalf("invalid performance samples: %v", err)
		}
		writeBaselineArtifacts(t, dir, samples)
		return
	}
	iterations := 5
	if raw := os.Getenv("MASK_RIVER_PERF_ITERS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			t.Fatalf("MASK_RIVER_PERF_ITERS=%q is not a positive integer", raw)
		}
		iterations = value
	}
	var samples []perfSample
	for _, cfg := range perfMatrix() {
		samples = append(samples, measureConfig(t, cfg, iterations)...)
	}
	if err := validateSamples(samples); err != nil {
		t.Fatalf("invalid performance samples: %v", err)
	}
	writeBaselineArtifacts(t, dir, samples)
	t.Logf("wrote %d samples to %s", len(samples), dir)
}

// TestPerfArtifactsMatchSamples re-derives every aggregated number from
// samples.json and compares it with what is on disk. Hand-editing a figure in
// summary.md or results.csv makes this test fail, which is the point: the report
// must not be a place where numbers are copied by hand.
func TestPerfArtifactsMatchSamples(t *testing.T) {
	dir := os.Getenv("MASK_RIVER_PERF_DIR")
	if dir == "" {
		t.Skip("set MASK_RIVER_PERF_DIR to check the artifacts")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "samples.json"))
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	var samples []perfSample
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatalf("parse samples: %v", err)
	}
	if err := validateSamples(samples); err != nil {
		t.Fatalf("invalid performance samples: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	regenDir := t.TempDir()
	rendered := writeBaselineArtifacts(t, regenDir, samples)
	if rendered != string(onDisk) {
		t.Fatal("summary.md does not match the numbers derivable from samples.json; regenerate it instead of editing it")
	}
	for _, name := range []string{"samples.json", "results.csv", "stats.csv", "environment.json"} {
		want, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		got, readErr := os.ReadFile(filepath.Join(regenDir, name))
		if readErr != nil {
			t.Fatalf("read regenerated %s: %v", name, readErr)
		}
		if string(want) != string(got) {
			t.Fatalf("%s does not match artifacts regenerated from samples.json", name)
		}
	}

	// The CSV must agree with the same computation, column by column.
	csvRaw, err := os.ReadFile(filepath.Join(dir, "results.csv"))
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(csvRaw)), "\n")
	if len(lines) < 2 {
		t.Fatalf("results.csv has %d lines", len(lines))
	}
	header := strings.Split(lines[0], ",")
	groups := perConfig(samples)
	for _, line := range lines[1:] {
		fields := strings.Split(line, ",")
		if len(fields) != len(header) {
			t.Fatalf("row %q has %d fields, want %d", line, len(fields), len(header))
		}
		key := fields[0] + "/" + fields[1] + "/w" + fields[2]
		group, ok := groups[key]
		if !ok {
			t.Fatalf("csv row %q has no matching samples", key)
		}
		var totals []float64
		for _, sample := range group {
			totals = append(totals, float64(sample.TotalNS))
		}
		want := statsOf(totals).Median
		got, err := strconv.ParseFloat(fields[5], 64)
		if err != nil {
			t.Fatalf("parse total_ns_median for %s: %v", key, err)
		}
		if difference(got, want) > 1 {
			t.Fatalf("%s: csv total_ns_median=%.0f, recomputed=%.0f", key, got, want)
		}
	}
}

func difference(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
