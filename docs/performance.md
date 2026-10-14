# Issue #13: bounded workers and repeatable synthetic benchmarks

## Implementation and safety boundary

PR #36 adds `--workers=1..16` to the **SQLite single-table/single-column** M1 CLI. The default is **one worker** because the observed small-batch workload did not benefit from extra workers. The Reader and Writer remain sequential; concurrency only transforms an already bounded (max 1,000 values) SQL page. A buffered channel has at most the worker count of queued jobs, and a fixed maximum of 16 workers is joined before return. Result slots preserve original row order. This does **not** parallelize transactions or imply an end-to-end speedup.

Deterministic tests compare one to 2/4/8/16 workers, and the real synthetic SQLite CLI fixture compares serial and four-worker durable database SHA-256 hashes, validation and uniqueness. Tests also inject a transform failure while other workers are active, cancel a running worker pool, and check all instrumented workers have exited before return. Go race checks execute in CI.

## Recorded actual benchmark evidence

The following is a **previous confirmed run**, not a prediction of the final PR Head:

- [GitHub Actions benchmark run 38331204874](https://github.com/haoyuehx/maskriver/actions/runs/38331204874), PR Head `ba17fa2eddb229e80e07bbd28f995cc7fb0d3e6c`.
- Environment: GitHub-hosted Ubuntu Linux x86_64, **Go 1.26.3**, Go reported `linux/amd64`; the exact runner hardware is transient. Benchmark command:
  `go test -run '^$' -bench '^BenchmarkMaskBatchWorkers$' -benchmem -count=3 -benchtime=300ms ./internal/runner`.
- Each benchmark operation performs HMAC-based synthetic `fake_email` masking of exactly **128** input values in memory. No real DB throughput, measured PII anonymization or security performance is implied.

| Workers | Median ns/batch (3 runs) | Range ns/batch | Median B/batch | Allocations/batch |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 250,007 | 248,211–289,273 | 130,428 | 2,818 |
| 2 | 339,605 | 337,855–379,040 | 131,178 | 2,827 |
| 4 | 288,029 | 285,671–288,944 | 131,467 | 2,829 |
| 8 | 266,182 | 264,175–322,833 | 132,027 | 2,833 |

The one-worker configuration was fastest in this run. The earlier [run 38266042530](https://github.com/haoyuehx/maskriver/actions/runs/38266042530) showed the same ordering. **Do not imply that more goroutines improve performance or that these short samples prove a speedup**.

The bench workflow archives `raw.txt`, `environment.txt`, `results.csv`, `results.json` and an SVG chart. An extended workflow also records per-process `/usr/bin/time -v` CPU seconds and peak RSS and machine-readable summaries; these numbers include Go test process startup/compilation/caching and are **not equivalent to steady-state mask-only RSS/CPU**. Do not claim full DB throughput until measured using actual database CLI workload sizes and distributions.

## Verification commands

```sh
gofmt -l internal/runner
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
go test -run '^$' -bench '^BenchmarkMaskBatchWorkers$' -benchmem -count=3 -benchtime=300ms ./internal/runner
go test -run '^$' -fuzz '^FuzzMaskBatchDeterminism$' -fuzztime=10s -parallel=2 ./internal/runner
```

The benchmark CI also runs bounded fuzz targets for CN identity/card/USCC checksum invariants using only synthetic seeds. Fuzz coverage does **not** prove global PII-format correctness.


## PR #37: real SQLite Runner workflow benchmark methodology

In addition to the PR #36 in-memory transformation benchmark, PR #37 adds a
real, disposable **SQLite** workflow benchmark via
`cmd/maskriver/sqlite_workflow_bench_test.go`. Each timed operation opens
two real database Readers, binds a frozen Plan, completes Preview, performs
optimistic SQL batched Apply, runs strict validation against the unchanged source
snapshot and closes the adapters. The synthetic setup and resetting the
target database file are excluded from the Go benchmark timer.

The fixed matrix is **128 / 512 / 2048 rows**, **short / long synthetic
email shapes**, and **1 / 4 transformation workers**, repeated **three**
times per configuration with `-benchtime=1x`. The CI captures `ns/op`,
`B/op`, `allocs/op`, and computed `rows/s` per case; its artifacts include
the raw output, environment metadata, CSV, JSON and a static SVG chart.
Results must be obtained from a successful real CI run, **not predicted**.
Run manually on a Linux host using the exact commands in
`.github/workflows/ci.yml` under `sqlite-db-benchmark`.

The process-level `/usr/bin/time -v` peak RSS and CPU seconds include
SQLite fixture creation and the entire compiled Go test binary process.
They are **not** per-workload sustained memory curves or evidence of bounded
resident memory at arbitrary input sizes. `TestSQLiteConnectionPoolsClose`
runs twenty real adapter close/cancel cycles and checks SQL pool statistics;
the isolated MySQL adapter test now checks a real socket-backed close.
These checks do not constitute a complete sustained MySQL load/leak benchmark.

**Remaining gates**: actual MySQL user workflow at several row counts and
distributions (not yet supported by the SQLite-only CLI), repeatable sustained
RSS/CPU measurements, independent non-author review, and upstream #9/#11 M1
acceptance. Keep #13 open until these are satisfied.

## Remaining Issue #13 acceptance

- Full end-to-end **SQLite and MySQL** throughput/latency comparisons at fixed multiple row counts and distributions; this PR measures only a 128-value in-memory HMAC task.
- CPU/RSS **under sustained whole-database workload** and memory scaling versus batch size and worker count.
- Non-author review, connection-leak instrumentation beyond local transform goroutines, and the blocked M1 #9/#11 safety acceptance.
- Any observed improvement must be justified by measured results instead of presumed parallel speedup.

Accordingly, the bounded worker implementation can merge independently without claiming all of Issue #13 is accepted or closing the issue prematurely.
