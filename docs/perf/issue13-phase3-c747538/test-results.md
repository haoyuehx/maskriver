# Final verification at source SHA `8ffca9b42ee3841d0d11e329b21a09e7c45aa682`

Host: Linux x86_64, kernel `7.0.0-38-generic`; Go `go1.26.3`; Ryzen 9 7940H (16 cores); host MemTotal 15,187 MiB (not container limit). The batch and observer measurements were captured at measurement code SHA `c747538edcc0fb52fb16a0eb796953cec1062c94`; `8ffca9b` adds post-marker independent Validate only to the strace test.

| Command/check | Result |
|---|---|
| `gofmt` on changed Go files | PASS before commit |
| `go test -count=1 ./...` | PASS, exit 0 |
| `go test -race -count=1 ./...` | PASS, exit 0 |
| `go vet ./...` | PASS, exit 0 |
| `go test -run '^$' -fuzz '^FuzzMaskBatchDeterminism$' -fuzztime=10s -parallel=2 ./internal/runner/` | PASS, 10 s, exit 0 |
| `GOOS=windows go test -c -o /tmp/maskriver-phase3-8ff.test.exe ./internal/runner` | PASS cross-compile; not executed on Windows |
| `go test -run '^$' -bench '^BenchmarkWorkflow/rows=128/shape=short/workers=1$' -benchmem -benchtime=2x ./internal/runner/` | PASS benchmark smoke |
| `MASK_RIVER_PHASE3_DIR=$PWD/docs/perf/issue13-phase3-c747538 go test -run '^TestPhase3ArtifactConsistency$' -count=1 ./internal/runner/` | PASS at later checkout; recorded environment provenance remains measurement code SHA c747538 |
| `MASK_RIVER_PHASE3_OVERHEAD_DIR=$PWD/docs/perf/issue13-phase3-c747538/observer-overhead go test -run '^TestPhase3ObserverOverheadArtifactConsistency$' -count=1 ./internal/runner/` | PASS |
| `MASK_RIVER_PERF_DIR=$PWD/docs/perf/issue13-phase2-62b4482 go test -run '^TestPerfArtifactsMatchSamples$' -count=1 ./internal/runner/` | PASS |
| `MASK_RIVER_STRACE_APPLY=1` marked test under `strace -f -ttt -T -yy -e trace=fsync,fdatasync,write` | PASS; full independent Validate after END marker |
| `git diff --check` | PASS |

Opt-in measurements passed: 30 randomized BatchSize trials (10 each); 10k rows × workers 1/2/4/8/16; 30 randomized paired observer comparisons (10 each plain/instrumented per BatchSize, 60 independent observations total); all artifact consistency checks passed. The high-load tests are manual-only; the existing PR benchmark job remains active on pull requests. For PR #68 at code head `e7088ae`, required GitHub checks passed on Ubuntu, Windows, isolated MySQL, and bench; the manual phase3-stress job was skipped by its `workflow_dispatch` guard.

Windows runtime, non-Linux resources, MySQL performance, CLI E2E performance, 100k/1M rows, and statistical significance remain UNVERIFIED/not measured.
