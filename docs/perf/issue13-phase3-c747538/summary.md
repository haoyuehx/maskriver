# Issue #13 Phase 3 — transaction observation (10 randomized repeats/config)

All rows are synthetic `@example.invalid`; each trial uses a fresh source/target and real Ed25519 approval. Fixture build is outside workflow/apply intervals. Apply has a complete receipt and passed independent full Validate on every row. See `samples.json` and `apply-transactions.csv` for every independent trial.

| Batch | commits/trial | Apply mean/median (ms) | min–max ms | CV % | Apply rows/s mean | Begin driver (ms) | Commit driver (ms) | UPDATE exec (ms) | Other SQL call (ms) | Transaction windows (ms) | Apply residual (ms) |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 16 | 129.90 / 130.26 | 124.07–136.37 | 3.3 | 1973 | 0.062 | 101.732 | 3.296 | 1.945 | 112.88 | 17.01 |
| 64 | 4 | 38.86 / 38.54 | 36.31–41.09 | 3.5 | 6594 | 0.014 | 23.626 | 1.277 | 0.406 | 28.13 | 10.74 |
| 256 | 1 | 16.10 / 16.28 | 13.56–18.39 | 9.6 | 16038 | 0.002 | 6.166 | 0.477 | 0.048 | 7.96 | 8.14 |

Apply wall time and rows/s were captured with SQLSpy and diagnostic counters enabled; they include observer overhead and are not production-throughput claims. The paired uninstrumented-vs-instrumented check is recorded in `observer-overhead/observer-summary.md`. Driver event timers cover the underlying driver calls and exclude post-call event recording. The event timers are nested observations, not mutually exclusive components: `writer.Begin` wrapper timing includes schema validation/queries plus driver Begin; `WriteTx.Update` includes row validation and driver UPDATEs; driver SQL timers are measured inside those calls. Do **not** add them. Apply remainder is unassigned and includes observer work as well as page reads, masking, receipt handling, and framework overhead; it must not be attributed solely to production work. This evidence does not claim all Apply time is commit/fsync. `Commit` invocation, driver call elapsed time and successful returned Commit are separately recorded; unknown/error is never called success.

Batch order is pseudorandomized with a fixed seed and each trial has a new DB/engine. Ten repeats are descriptive; no significance claim is made. `strace` attribution is in `fsync-evidence.md`.
