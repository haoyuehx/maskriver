# Issue #13 WP3-F — real CLI streaming scale evidence

**Head:** `41e96b18130d9becfabb15d2bca502054331c1cd`
**Status:** measured, synthetic, not an approved production budget.

Each sample runs the real CLI child process: `scan`, `preview`, `approve --streaming`,
`mask --streaming --apply`, `validate --strict`. The source is a pristine synthetic file; the
target is re-copied for every sample; every sample uses its own RunID. Fixture generation and the
file copy are outside the timed phases.

## Results (median per cell)

| Rows | Workers | Batch | Samples | Apply ms | Rows/s | Total ms | Apply peak RSS | Outbox bytes |
|---|---|---|---|---|---|---|---|---|
| 10,000 | 1 | 64 | 5 | 1,409 | 7,097 | 1,663 | 24.1 MiB | 663,552 |
| 10,000 | 4 | 64 | 5 | 1,457 | 6,863 | 1,710 | 24.0 MiB | 663,552 |
| 10,000 | 4 | 256 | 5 | 553 | 18,083 | 806 | 24.6 MiB | 634,880 |
| 100,000 | 1 | 64 | 3 | 13,522 | 7,395 | 14,530 | 37.8 MiB | 6,639,616 |
| 100,000 | 4 | 64 | 3 | 13,669 | 7,316 | 14,825 | 37.4 MiB | 6,639,616 |
| 100,000 | 4 | 256 | 3 | 4,874 | 20,517 | 6,032 | 37.7 MiB | 6,352,896 |
| 1,000,000 | 1 | 64 | 3 | 133,067 | 7,515 | 142,463 | 39.2 MiB | 66,473,984 |
| 1,000,000 | 4 | 64 | 3 | 134,763 | 7,420 | 145,585 | 38.8 MiB | 66,478,080 |
| 1,000,000 | 4 | 256 | 3 | 48,560 | 20,593 | 58,167 | 38.8 MiB | 63,627,264 |

Raw data: `cli-streaming-scale.csv` (33 samples). No p50/p95 is computed: 3 samples are too few for a
percentile claim, and medians are reported as descriptive only.

## Observations, restricted

- **Peak RSS is flat in row count per configuration.** 10k ≈ 24 MiB, 100k ≈ 38 MiB, 1M ≈ 39 MiB.
  The increase from 10k to 100k is runtime/SQLite baseline plus paging, not linear in N.
- **Batch size, not worker count, dominates throughput.** 256-row batches are ~2.7× faster than 64;
  workers 1 and 4 are within noise because the single serialized commit dominates.
- **Outbox grows with committed batches.** 1M rows at batch 64 = 15,625 receipts ≈ 63–66 MiB;
  batch 256 = 3,907 receipts ≈ 64 MiB. This is disk growth, not heap growth.
- **Every sample reported `state=complete`, `audit=consistent`, `rows_committed` equal to the
  dataset size, and `batches_unknown=0`.** Strict validate passed.
- RSS is process-wide and sampled every 50 ms; very short phases (scan/approve/validate) are
  deliberately not reported. Heap and TotalAlloc are not available for the child process.

## Not verified

- MySQL/Windows streaming runtime.
- Power-loss durability, mid-commit kill, receipt-time ENOSPC.
- File-handle-level TOCTOU protection.
- Any claim that MaxProcessBytes is an OS hard cap: it is an admission rule only.
