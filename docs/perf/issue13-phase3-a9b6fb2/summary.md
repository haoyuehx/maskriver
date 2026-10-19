# Issue #13 Phase 3 — transaction observation (10 randomized repeats/config)

All rows are synthetic `@example.invalid`; each trial uses a fresh source/target and real Ed25519 approval. Fixture build is outside workflow/apply intervals. Apply has a complete receipt and passed independent full Validate on every row. See `samples.json` and `apply-transactions.csv` for every independent trial.

| Batch | commits/trial | Apply mean/median (ms) | min–max ms | CV % | Apply rows/s mean | Begin driver (ms) | Commit driver (ms) | UPDATE exec (ms) | Other SQL call (ms) | Transaction windows (ms) | Apply residual (ms) |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 16 | 131.59 / 131.40 | 126.57–136.69 | 2.4 | 1946 | 0.070 | 102.231 | 3.472 | 1.989 | 113.88 | 17.72 |
| 64 | 4 | 39.43 / 39.50 | 38.23–40.64 | 2.8 | 6498 | 0.016 | 24.744 | 1.303 | 0.461 | 29.37 | 10.05 |
| 256 | 1 | 16.17 / 16.24 | 14.22–17.25 | 4.7 | 15866 | 0.003 | 5.885 | 0.527 | 0.068 | 7.86 | 8.31 |

The event timers are nested observations, not mutually exclusive components: `writer.Begin` wrapper timing includes schema validation/queries plus driver Begin; `WriteTx.Update` includes row validation and driver UPDATEs; driver SQL timers are measured inside those calls. Do **not** add them. Apply remainder is unassigned and may include page reads, masking, receipt handling, and framework overhead. This evidence does not claim all Apply time is commit/fsync. `Commit` invocation, driver call elapsed time and successful returned Commit are separately recorded; unknown/error is never called success.

Batch order is pseudorandomized with a fixed seed and each trial has a new DB/engine. Ten repeats are descriptive; no significance claim is made. `strace` attribution is in `fsync-evidence.md`.
