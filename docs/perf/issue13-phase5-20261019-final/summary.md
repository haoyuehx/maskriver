# Issue #13 Phase 5 — SQLite scale, resource, and platform evidence

**Verdict: INSUFFICIENT EVIDENCE TO CLOSE ISSUE #13.** This is a diagnostic-only test/evidence change. No production optimization was selected or shipped. Keep #13 OPEN; do not start #14.

## Scope and guardrails

- Base commit: `050fde78e25edfad38f5bd47119168eaf5eb2ada` (PR #69 merge); branch: `perf/issue13-phase5-scale-platform-final-20261019`.
- Frozen `pkg/contracts.APIRevision=m1a-v1`, release `v0.1.0-dev`, production BatchSize default (64), transaction behavior, and production code were not changed.
- Synthetic `.invalid` values only. Each trial creates a fresh SQLite source, copies it to a separate target, creates a temporary synthetic Ed25519 approval store, then removes the fixture and approval data. No database, key, raw field value, or profile is archived.
- Harness sources and exact SHA-256 provenance are in `source-provenance.json`; environment and preflight are in `environment.json` and `preflight.json`. Raw data is in `samples.json`, `samples.ndjson`, and `resource-samples.csv`; per-trial files are under `raw/`; `rollup.csv` is a summary from the raw samples.

## Method

`TestPhase5ScaleMatrix` launched 42 isolated Linux test processes against the real SQLite Runner: 10k rows × workers 1/2/4/8/16 × 5 repetitions; 100k × 1/2/4/8/16 × 3; 1M × workers 1/4 × 1. Every trial used the unchanged BatchSize=64. Fixture construction (one prepared source transaction and a closed-file copy to target) is timed separately. The measured workflow is Scan → Plan → Preview → real signed Approve → Apply → Validate. Independent checks then compare source/target aggregates, target logical digests, receipt/commit counts, source file hashes, NULL/distinct counts, and cross-worker target digests.

Both columns are synthetic and present in every fixture. `email_high_cardinality` uses `h%07d@example.invalid` modulo 9,500 (no NULL); `email_repeated_null` is NULL for row indexes divisible by 20 and otherwise uses `r%03d@example.invalid` modulo 100 (5% NULL for these row counts, 95 distinct non-NULL values). Actual source aggregates are checked against these declared fixture parameters. Sampling runs every 100 ms from after fixture creation through workflow and independent SQL verification. RSS/heap/goroutine/FD values are sampled process values; database/sql pool values are sampled and also checked synchronously around writes and close. This is an instrumented Go test process, **not a CLI process**. `runner_test_process_wall_ns` includes test-process startup; it is not workflow latency.

The matrix is sequential and uses no cache flush/randomized order or control-group A/B candidate. The harness is diagnostic, its sampling adds overhead, and its results are machine-specific. Go allocation counts/B/op are not collected for these end-to-end cells; sampled heap and receipt capacity are not substitutes. Do not treat these numbers as production throughput or statistically significant speedups. Repetition counts are too small for a meaningful p95; report medians and observed ranges only.

## Results

Workflow and Apply medians are milliseconds; rows/s is the median per-trial workflow throughput. RSS and receipt memory are maxima within each group. Times are descriptive only.

| Rows | Workers | Trials | Workflow median (min–max) | Apply median | Workflow rows/s median | Sampled RSS max | Receipt slice capacity max | Successful commits/trial |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 10k | 1 | 5 | 2,121 (2,107–2,134) | 1,474 | 4,716 | 29.8 MiB | 0.019 MiB | 157 |
| 10k | 2 | 5 | 2,192 (2,181–2,211) | 1,521 | 4,562 | 29.6 MiB | 0.019 MiB | 157 |
| 10k | 4 | 5 | 2,170 (2,151–2,179) | 1,514 | 4,608 | 30.0 MiB | 0.019 MiB | 157 |
| 10k | 8 | 5 | 2,171 (2,159–2,179) | 1,510 | 4,606 | 30.0 MiB | 0.019 MiB | 157 |
| 10k | 16 | 5 | 2,180 (2,150–2,189) | 1,511 | 4,588 | 30.2 MiB | 0.019 MiB | 157 |
| 100k | 1 | 3 | 15,180 (15,162–15,213) | 13,487 | 6,588 | 36.7 MiB | 0.133 MiB | 1,563 |
| 100k | 2 | 3 | 15,838 (15,831–15,844) | 13,932 | 6,314 | 35.3 MiB | 0.133 MiB | 1,563 |
| 100k | 4 | 3 | 15,636 (15,576–15,702) | 13,810 | 6,395 | 36.5 MiB | 0.133 MiB | 1,563 |
| 100k | 8 | 3 | 15,614 (15,601–15,681) | 13,829 | 6,405 | 36.4 MiB | 0.133 MiB | 1,563 |
| 100k | 16 | 3 | 15,671 (15,645–15,776) | 13,864 | 6,381 | 36.8 MiB | 0.133 MiB | 1,563 |
| 1M | 1 | 1 | 145,856 (single sample) | 133,691 | 6,856 | 37.3 MiB | 1.102 MiB | 15,625 |
| 1M | 4 | 1 | 150,716 (single sample) | 137,503 | 6,635 | 37.1 MiB | 1.102 MiB | 15,625 |

Per-size median stage times across the worker groups (milliseconds):

| Rows | Fixture | Scan | Plan | Preview | Approve | Apply | Validate | Independent SQL/digest checks |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 10k | 34.4 | 137.0 | 135.9 | 208.3 | 12.2 | 1,510.4 | 166.8 | 26.9 |
| 100k | 175.5 | 181.4 | 185.0 | 929.3 | 12.1 | 13,853.0 | 495.2 | 233.1 |
| 1M | 1,535.4 | 627.6 | 669.5 | 7,566.2 | 12.2 | 135,597.0 | 3,813.6 | 2,209.4 |

Approximate process CPU during the Scan-through-Validate workflow (the verification stage is excluded):

| Rows | User CPU median | System CPU median | Total CPU median (observed range) |
|---:|---:|---:|---:|
| 10k | 1.51 s | 0.23 s | 1.70 s (1.43–1.95) |
| 100k | 8.07 s | 1.87 s | 9.94 s (7.38–11.93) |
| 1M | 68.43 s | 15.14 s | 83.57 s (70.29–96.84) |

All 42 trials completed, passed Validate and aggregate checks, preserved the source file SHA-256, and had expected receipts/commits. Target NULL and distinct counts matched the source; every high-cardinality row and every non-NULL repeated-field row was changed. Cross-worker target logical digests matched. The sampler recorded 5,999 resource rows. Maximum sampled RSS was 37.3 MiB, sampled heap allocation 5.12 MiB, sampled goroutines 22, and sampled file descriptors 12. Reader, target and writer pools each peaked at one open connection; the test asserted reader and target pools closed with zero open connections, and recorded writer open count zero after close in every trial. Post-close goroutines returned to within two of each trial's pre-sampler baseline.

The receipt slice is retained in `ApplyReport`, and its measured backing capacity increases with the number of batches: 0.019 MiB at 10k, 0.133 MiB at 100k, and 1.102 MiB at 1M. Therefore this evidence **does not establish the strict AC that total memory is bounded only by batch × workers**. RSS samples remained modest in this workload, but are not a proof of that invariant. Addressing accumulated receipts would require explicit shared-contract review; this phase did not alter the frozen contract.

No worker count demonstrated a repeatable gain here. One worker had the lowest median at 10k; the 100k medians vary by worker without a repeatable monotonic benefit; at 1M, the single 1-worker trial was faster than the single 4-worker trial. The differences do not justify changing worker defaults, BatchSize, transaction policy, or SQL implementation.

## Acceptance audit

| Original #13 acceptance area | Phase-5 evidence | Status |
|---|---|---|
| Bounded, cancellable, deterministic concurrency | Existing worker-pool tests and cancellation/fault-injection tests pass; 1–16 worker output digests agree in the measured sizes | Partial: no claim that all total memory is bounded; receipts grow with batch count |
| Race, fuzz, leak and failure tests | Full race suite, four short fuzz targets, focused cancellation/failure, Preview zero-write, connection-close, and CLI tests pass (commands below) | Passed for the executed tests |
| Synthetic-only / no value or key leakage | Synthetic reserved values only; aggregate/hash/resource artifacts contain no fixture rows or key | Passed for this evidence |
| Method and measured results documented; matrix updated truthfully | This report, source provenance, raw samples, rollup and resource CSV; Feature Matrix remains partial and links this evidence | Passed for measured scope |
| Reproducible command, environment, repeated distribution, correctness, explicit gaps | 42-cell SQLite Runner command and environment/raw output archived | Partial: 1M has one repetition; no randomized order or uninstrumented control |
| Scale CLI E2E | Existing SQLite CLI E2E tests pass, but the 10k–1M matrix calls Runner APIs directly | Not measured |
| Windows/macOS native runtime performance | Windows and macOS test binaries cross-compiled; no native execution available here | Unverified |
| MySQL CLI E2E performance | Project has no MySQL CLI; no MySQL CLI run is claimed | Not applicable / not measured |
| Full memory bound by batch × workers | Receipt backing capacity grows with total batches | Not satisfied by this evidence |

## Commands and platform

Measurement host: Linux/amd64, Go 1.26.3, 16 logical CPUs, AMD Ryzen 9 7940H, Linux 7.0.0-38-generic, modernc.org/sqlite v1.60.1. Preflight measured 160,813,273,088 available disk bytes and 9,406,084 KiB `MemAvailable`; no cgroup `memory.max` file was exposed. The reported SQLite default journal mode was DELETE; the test did not change synchronous settings.

- Scale matrix: `MASKRIVER_PHASE5_SCALE_OUT="$PWD/docs/perf/issue13-phase5-20261019-final" go test -run '^TestPhase5ScaleMatrix$' -count=1 -timeout 4h ./internal/runner` — PASS, 42 samples, 600.463 s.
- `go test -count=1 ./...` — PASS.
- `go test -race -count=1 ./...` — PASS.
- `go vet ./...` — PASS.
- Focused runner cancellation/failure/approval/Preview-write-safety tests and `cmd/maskriver` approval/scan-failure CLI tests — PASS.
- `FuzzMaskBatchDeterminism`, `FuzzCNIDReplacement`, `FuzzCNBankReplacement`, `FuzzCNUSCCReplacement`, each with `-fuzztime=2s` — PASS.
- Windows/amd64 and Darwin/amd64 test packages cross-compiled with `go test -run '^$' -exec /bin/true ./...` — build PASS only; these commands did not execute native tests.
- MySQL adapter and MySQL CLI measurements — not run locally; no MySQL CLI exists. Isolated MySQL adapter CI remains separate component evidence, not a CLI benchmark.

## Remaining work / decision

Do not close #13. Remaining evidence includes statistically repeated 1M trials, allocation/B-op measurements, continuous uninstrumented and native CLI-scale measurements, native Windows/macOS runtime coverage, additional sustained resource/lock/WAL/cancellation measurements, and a resolution (if needed) for receipt memory scaling that respects the frozen contract. The original performance problem is not demonstrated by these runs, so no optimization is selected. No production defaults, transaction behavior, contracts or release artifacts were changed.
