# Issue #14 recovery scale evidence (09612f8)

**Implementation head:** `09612f8` (Issue #14 review-fixed head)
**Date:** 2026-10-21
**Environment:** Linux amd64, Go 1.26.3, `modernc.org/sqlite v1.60.1`,
`journal_mode=delete`, session `synchronous=FULL`, BatchSize 256, Workers 4.
Synthetic `people(id, email)` data only; no production data.

Command (one row count at a time):

```sh
MASKRIVER_ISSUE14_SCALE=1 \
MASKRIVER_ISSUE14_SCALE_DIR="$PWD/docs/perf/issue14-09612f8" \
MASKRIVER_ISSUE14_ROWS=10000,100000,1000000 \
go test -count=1 -run TestCLIRecoveryScaleEvidence -v ./cmd/maskriver
```

Every scenario is a real CLI child process. The baseline is an uninterrupted
Apply on an identical dataset. The interrupted scenario kills the real process
after a deterministic number of committed batches (`after_commit=N`, test-only
hook), then runs `status` and a separate `--resume` child on the same files.

## Results

| Rows | Scenario | Interrupt after | Baseline apply | Interrupted apply | Resume | Resume / baseline apply | Validate | Peak RSS (interrupted / resume) | Rows / batches | Checkpoint / progress / receipts | A/B digest equal | Strict validate |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :---: | :---: |
| 10,000 | baseline | — | 1,470 ms | — | — | — | 203 ms | — | 10,000 / 40 | 41 / 41 / 40 | — | PASS |
| 10,000 | interrupted+resume | 20 | (same) | 808 ms | 862 ms | 0.586 | 153 ms | 22,400 / 23,960 KiB | 10,000 / 40 | 41 / 41 / 40 | yes | PASS |
| 100,000 | baseline | — | 12,383 ms | — | — | — | 1,065 ms | — | 100,000 / 391 | 392 / 392 / 391 | — | PASS |
| 100,000 | interrupted+resume | 195 | (same) | 7,382 ms | 7,134 ms | 0.576 | 1,051 ms | 31,128 / 34,552 KiB | 100,000 / 391 | 392 / 392 / 391 | yes | PASS |
| 1,000,000 | baseline | — | 109,680 ms | — | — | — | 8,904 ms | — | 1,000,000 / 3,907 | 3,908 / 3,908 / 3,907 | — | PASS |
| 1,000,000 | interrupted+resume | 1,953 | (same) | 59,331 ms | 67,145 ms | 0.612 | 8,320 ms | 35,304 / 38,796 KiB | 1,000,000 / 3,907 | 3,908 / 3,908 / 3,907 | yes | PASS |

Raw CSVs: `cli-recovery-scale-10k-100k.csv`, `cli-recovery-scale-1m.csv`.
`environment.json` records the platform and budget.

## Interpretation and limits

- Resume re-verifies the committed prefix and the uncommitted suffix, so it is
  **not** a cheap "seek": the measured resume cost is about 0.58–0.61× the
  uninterrupted Apply on these synthetic shapes. That is expected for a design
  that recomputes replacements instead of trusting a local cursor file.
- Peak RSS stayed under 40 MiB at 1M rows, consistent with bounded pages. The
  measurement is `/proc/<pid>/status` `VmHWM` sampled every 50 ms; it is not a
  Go-heap-only figure and does not prove a strict batch×workers bound.
- 1M is a single sample on this machine, not a statistical distribution. No
  production throughput or exactly-once claim follows.
- `target_bytes` is only populated for the interrupted target (the masked file
  containing outbox + recovery metadata): 663,552 bytes at 10k, 6,426,624 at
  100k and 64,221,184 at 1M. Recovery metadata grows with the number of batches
  and is never auto-dropped.
- The scale test is opt-in (`MASKRIVER_ISSUE14_SCALE=1`) and is not run in CI;
  CI runs the deterministic fault-injection and correctness suites.
