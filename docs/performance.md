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

The bench workflow archives `raw.txt`, `environment.txt`, `results.csv`, `results.json` and an SVG chart. An extended workflow also records per-process `/usr/bin/time -v` CPU seconds and peak RSS and machine-readable summaries; these numbers include Go test process startup/compilation/caching and are **not equivalent to steady-state mask-only RSS/CPU**.

## Phase-5 real SQLite Runner scale evidence (2026-10-19)

[The reproducible phase-5 report](perf/issue13-phase5-20261019-final/summary.md) records 42 real-SQLite Runner trials at 10k/100k/1M synthetic rows, workers 1/2/4/8/16 where repetitions were budgeted, fixed BatchSize 64, per-stage timings, rows/s, sampled process resources, SQL pool closure, receipt counts, source immutability and independent Validate/aggregate checks. It is **Runner API coverage, not CLI scale E2E**, and its sequential/instrumented measurements do not establish a production speedup. The 1M cases were single samples. Receipt slice capacity grows with total batches, so these runs do not prove the strict batch×workers memory bound. No production optimization or default change was justified. Issue #13 remains open.

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

## Remaining Issue #13 acceptance

- Full-scale **SQLite CLI** comparisons remain unmeasured: phase 5 exercises the Runner API and existing CLI tests are not a 10k–1M performance matrix. The project has no MySQL CLI; MySQL adapter component tests are not MySQL user-workflow benchmarks.
- Statistically repeated 1M trials, uninstrumented workload comparison, native Windows/macOS runtime performance, additional sustained lock/WAL/cancellation metrics, and evidence that total memory is strictly bounded by batch × workers (receipt capacity currently grows with batch count).
- Final-head independent QA/Security review and closure of every remaining original Issue #13 acceptance item.
- Any observed improvement must be justified by measured results instead of presumed parallel speedup.

Accordingly, the bounded worker implementation can merge independently without claiming all of Issue #13 is accepted or closing the issue prematurely.

## Issue #14 local recovery scale evidence (2026-10-21)

[The recovery scale report](perf/issue14-09612f8/summary.md) records real CLI
child-process runs at 10k/100k/1M synthetic rows with BatchSize 256 and Workers 4:
an uninterrupted baseline and a deterministic half-way interruption followed by a
separate `--resume` process. At 1M the baseline Apply was 109.7 s, resume was
67.1 s (0.612× the baseline Apply), strict Validate passed, Peak RSS was 34.5 MiB
for the interrupted process and 37.9 MiB for resume, and the A/B ordered-email
SHA-256 matched. Resume re-verifies the committed prefix and uncommitted suffix,
so it is not a cheap cursor seek. These are single-machine synthetic samples, not
production throughput; the scale test is opt-in and not part of CI.
