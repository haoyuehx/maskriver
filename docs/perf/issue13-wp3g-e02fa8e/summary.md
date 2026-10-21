# WP3-G CLI scale evidence (byte-aware paging)

Code head `e02fa8e595a5ff4b008a173ed8e146e75794d4b6`. Raw data: `cli-streaming-scale.csv`;
environment and method: `environment.json`. All values below are generated from the CSV;
medians are over 3 samples per cell. No p95 is claimed from 3 samples.

| Rows | Workers | Batch | Apply median ms | Rows/s | Total median ms | Apply peak RSS MiB | Outbox bytes |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 10,000 | 1 | 64 | 2,367 | 4,225 | 2,721 | 24.4 | 663,552 |
| 10,000 | 4 | 64 | 2,368 | 4,223 | 2,722 | 24.5 | 663,552 |
| 10,000 | 4 | 256 | 956 | 10,460 | 1,311 | 24.4 | 634,880 |
| 100,000 | 1 | 64 | 15,420 | 6,485 | 16,530 | 37.7 | 6,639,616 |
| 100,000 | 4 | 64 | 15,962 | 6,265 | 17,120 | 38.0 | 6,639,616 |
| 100,000 | 4 | 256 | 5,176 | 19,320 | 6,285 | 37.7 | 6,348,800 |
| 1,000,000 | 1 | 64 | 146,116 | 6,844 | 155,465 | 39.0 | 66,473,984 |
| 1,000,000 | 4 | 64 | 168,133 | 5,948 | 178,596 | 39.0 | 66,473,984 |
| 1,000,000 | 4 | 256 | 62,102 | 16,103 | 72,819 | 39.5 | 63,627,264 |

## Observations

- Apply peak RSS is flat in row count: about 24 MiB at 10k, 38 MiB at 100k and 39 MiB at 1M, matching the constant-size page/worker/outbox working set.
- Batch 256 is about 2.5-2.8x faster than batch 64 at 100k and 1M; workers 1 vs 4 is within run-to-run noise, as in WP3-F.
- Compared with WP3-F (`docs/perf/issue13-wp3f-41e96b1/`, same machine class, different day), apply wall time is 63-73% higher at 10k, 6-17% higher at 100k and 10-28% higher at 1M (throughput 38-42% lower at 10k, 6-14% lower at 100k, 9-22% lower at 1M). The delta is consistent with the extra same-snapshot length preflight, the replacement-row check and per-page transaction cost; no throughput target is claimed.
- Every sample reported `rows_committed` equal to the dataset size, complete state, consistent audit and a passing strict Validate (enforced by the harness).

## Not verified

- MySQL and Windows streaming runtime.
- p95/p99 latency, sustained multi-process concurrency and power-loss durability.
- Child-process heap/TotalAlloc; only sampled VmHWM is available.
