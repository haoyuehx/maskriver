# Issue #13 Phase 3 — transaction observation (10 randomized repeats/config)

All rows are synthetic `@example.invalid`; each trial uses a fresh source/target and real Ed25519 approval. Fixture build is outside workflow/apply intervals. Apply has a complete receipt and passed independent full Validate on every row. See `samples.json` and `apply-transactions.csv` for every independent trial.

| Batch | commits/trial | Apply mean/median (ms) | min–max ms | CV % | Apply rows/s mean | Begin driver (ms) | Commit driver (ms) | UPDATE exec (ms) | Other SQL call (ms) | Transaction windows (ms) | Apply residual (ms) |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 16 | 123.68 / 124.23 | 116.26–132.52 | 3.9 | 2073 | 0.057 | 96.890 | 3.026 | 1.573 | 106.90 | 16.78 |
| 64 | 4 | 38.35 / 38.31 | 36.17–40.26 | 3.2 | 6682 | 0.013 | 24.432 | 1.167 | 0.363 | 28.60 | 9.75 |
| 128 | 2 | 23.94 / 24.24 | 22.34–26.42 | 5.1 | 10718 | 0.006 | 11.661 | 0.725 | 0.134 | 14.39 | 9.55 |
| 256 | 1 | 16.89 / 16.38 | 14.19–20.24 | 9.9 | 15282 | 0.003 | 5.803 | 0.554 | 0.047 | 7.92 | 8.97 |

Apply wall time and rows/s were captured with SQLSpy and diagnostic counters enabled; they include observer overhead and are not production-throughput claims. Paired plain/instrumented samples are in `observer-paired-samples.csv`. Driver event timers are nested observations, not mutually exclusive components; do not add them. Apply remainder is unassigned and includes observer work, page reads, masking, receipt handling, and framework overhead. No phase-4 fsync trace was collected; the historical phase-3 trace is not attributed to these runs.
