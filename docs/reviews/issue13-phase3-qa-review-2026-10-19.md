# Issue #13 phase-3 independent QA review — 2026-10-19

## Verdict

**FAIL (review gate blocked).** The correctness and race suites pass, the raw 256-row samples reconcile, and a fresh 10,000-row run passes all runtime correctness invariants. However, the required artifact-consistency command fails at the exact review HEAD because committed `environment.json` identifies `a9b6fb267a414b83f4fa10a758de504843763bf0`, while regeneration records the current exact HEAD `1e5b5b0728ea833063b959fd0d7c2f98c9ad5503`. The checked-in phase-3 artifact set therefore does not pass its own consistency gate at the requested review SHA.

The fresh 10k output also differs byte-for-byte from all four checked-in 10k files. The correctness fields are unchanged and valid; the differences are the expected newly observed timings/resources plus the HEAD SHA. Per the task's stop condition, no checked-in evidence was rewritten and no further performance rerun was attempted.

## Scope and environment

- Isolated worktree: `/home/haoyue/Project/worktrees/maskriver/issue13-phase3-qa-1e5b5b0`
- Git common dir: `/home/haoyue/Project/maskriver/.git`
- Detached review HEAD / requested branch tip: `1e5b5b0728ea833063b959fd0d7c2f98c9ad5503` (`perf/issue13-phase3-transaction-cost-20261019`)
- Baseline before phase 3: `7244ba5`; implementation commits `ad4a746`, `31056c1`, `ecd6538`, `a9b6fb2`; evidence commit `1e5b5b0`
- OS: Linux x86_64, kernel `7.0.0-38-generic`
- Go: `go1.26.3 linux/amd64`
- Evidence reviewed: `docs/perf/issue13-phase3-a9b6fb2/`
- Requirements read: `AGENTS.md`, `docs/architecture.md`, `docs/contracts.md` §7, `docs/test-plan.md`, and `docs/parallel-tasks.md`
- Synthetic-data boundary: fixture descriptions use `@example.invalid`; the metric JSON contains no row-value fields, DSN, private-key field, seed-value field, original-value field, or replacement-value field. No network or external database was used.

## Commands and results

| Command | Exit/result | Notes |
|---|---:|---|
| `go test -count=1 ./...` | 0 / PASS | All packages passed; `internal/runner` 27.187s. Worktree remained clean. Ordinary test output did not enumerate opt-in MySQL execution, so this is not a new real-MySQL performance claim. |
| `go test -race -count=1 ./...` | 0 / PASS | All packages passed under race; `internal/runner` 32.755s. Worktree remained clean. |
| `MASK_RIVER_PHASE3_DIR=$PWD/docs/perf/issue13-phase3-a9b6fb2 go test -run '^TestPhase3ArtifactConsistency$' -count=1 ./internal/runner/` | 1 / FAIL | `phase3_artifacts_test.go:39: environment.json is not reproducible from samples.json`. No tracked file changed. |
| `MASK_RIVER_PHASE3_10K_DIR=/tmp/maskriver-phase3-qa-10k go test -run '^TestPhase3Scale10K$' -count=1 ./internal/runner/` | 0 / PASS | Used the supervisor-authorized external output directory to avoid rewriting evidence. Equivalent test/configuration; completed in 10.998s. Generated four files under `/tmp`. Worktree remained clean. |
| Independent Python reconciliation of `samples.json`, `apply-transactions.csv`, `apply-batches.csv`, committed 10k JSON, and regenerated 10k JSON | 0 / PASS | Assertions and totals are recorded below. |
| `diff -u` for each committed/generated 10k file and `sha256sum` | differences found | All four 10k files differ; exact field differences and hashes are below. |

Command logs were captured outside the repository at `/tmp/maskriver-qa-go-test.log`, `/tmp/maskriver-qa-go-race.log`, `/tmp/maskriver-qa-artifacts.log`, and `/tmp/maskriver-qa-10k.log`.

## Independent evidence checks

### Raw 256-row phase-3 samples

Evidence locations: `samples.json`, `resource-metrics.json`, `apply-transactions.csv`, `apply-batches.csv`, `results.{csv,json}`, `stats.csv`, and `summary.md`.

- Exactly 30 samples: 10 each for batch sizes 16, 64, and 256; every sample has 256 rows and 4 workers.
- Expected receipts/commits per trial are respectively 16, 4, and 1.
- Batch 16 totals: 160 successful commits, 160 receipts, 2,560 committed rows.
- Batch 64 totals: 40 successful commits, 40 receipts, 2,560 committed rows.
- Batch 256 totals: 10 successful commits, 10 receipts, 2,560 committed rows.
- Across all configurations: 30 transaction CSV rows, 210 batch CSV rows, 210 successful commits, 210 receipts, 7,680 committed rows, 7,680 receipt rows, and 7,680 UPDATE statements.
- Every raw trial records `complete=true`, `independent_validate_passed=true`, no begin failure, no commit error/unknown, no rollback attempt, exactly 256 receipt rows, exactly 256 updates, and reader/writer open counts 0/0 after close.
- Per-batch SQL/receipt arrays have the expected receipt length; statement, timing, UPDATE, and receipt totals reconcile to each parent trial. The repository's independent validator also accepts the raw sample invariants before failing only the regenerated environment file comparison.

### 10,000-row matrix

Evidence locations: committed `10k-workers.{json,csv}`, `10k-summary.md`, `10k-environment.json`; fresh output `/tmp/maskriver-phase3-qa-10k/`.

Both committed and freshly generated JSON contain exactly workers `1,2,4,8,16`. For every worker count:

- target rows = 10,000;
- committed rows = 10,000;
- receipts = 157 (`ceil(10000/64)`);
- `complete`, `validate_passed`, and `source_unchanged` are true;
- reader/writer connections after close are 0/0.

The fresh test itself compares each worker's complete primary-key-to-output map with workers=1 via `sameRows`; exit 0 therefore independently confirms exact per-key output equality for this run. It also checks full validation, exact target/receipt counts, source-file digest stability, connection closure, and fixture-directory cleanup before writing results.

### Claims and statistical limits

- `summary.md` explicitly says ten repeats are descriptive and makes no significance claim.
- `10k-summary.md` explicitly says one observation per worker count is descriptive and makes no significance or speedup claim.
- `fsync-evidence.md` limits the finding to strong SQLite-file correlation and explicitly declines exclusive per-Commit causality.
- `docs/perf/issue13-phase2-report.md` describes observed timing shares and says no significance is claimed. Its statement that increased workers showed no consistent speedup is a description of the five single observations, not a statistical conclusion.
- The evidence does not support generalization to CLI E2E, MySQL performance, non-Linux resource behavior, 100k/1M scale, production data, or statistical significance; those limits are stated in the report.

## Exact 10k differences

No correctness/status/count field changed. Exact changed values (`committed -> fresh`) are:

| Workers | fixture ms | workflow ms | apply ms | rows/s | CPU user/system ms | RSS before→after KB |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 60.52 → 48.11 | 1917.32 → 1958.69 | 1437.95 → 1459.19 | 5215.61 → 5105.45 | 1059.28/129.60 → 1084.55/102.35 | 19320→25752 → 18968→25284 |
| 2 | 52.93 → 54.64 | 1963.28 → 1954.50 | 1469.87 → 1462.01 | 5093.52 → 5116.39 | 1247.05/170.45 → 1240.38/159.25 | 25880→25632 → 25920→25768 |
| 4 | 62.16 → 53.07 | 1950.69 → 1968.99 | 1459.10 → 1474.47 | 5126.39 → 5078.73 | 1324.92/147.70 → 1314.48/155.74 | 26308→25952 → 25712→25852 |
| 8 | 51.86 → 60.12 | 1971.46 → 1977.95 | 1475.61 → 1491.14 | 5072.38 → 5055.73 | 1386.27/165.28 → 1363.96/192.64 | 26664→26096 → 26740→25816 |
| 16 | 59.70 → 60.18 | 1969.10 → 1972.61 | 1485.27 → 1481.36 | 5078.45 → 5069.42 | 1452.91/193.69 → 1438.53/199.75 | 27196→27824 → 27032→27120 |

Nanosecond-exact values are preserved in `/tmp/maskriver-phase3-qa-10k/10k-workers.json`; the unified diff also showed only those timing/resource fields in worker JSON, their rendered equivalents in CSV/summary, and `head_sha` in environment JSON. `10k-environment.json` changed `a9b6fb267a414b83f4fa10a758de504843763bf0` to `1e5b5b0728ea833063b959fd0d7c2f98c9ad5503`.

SHA-256 (`committed -> fresh`):

- `10k-environment.json`: `ee60a44a4070c255f18d750d54bb83b0df5556add5260795633deff9efe38d06` → `5511e695000f60bf108c4a287672b342608e93641b1c3a44a5cc2efbd3a5cd1c`
- `10k-summary.md`: `0b7a2a94b1aa4ac12c3206fcad320f4e302cb2b0a86ea679f04c527a2cae3f2b` → `b60667b9136ad755c141b0185d82d4fc5ebfe702a83373ed366374cb3693a0d3`
- `10k-workers.csv`: `9f0cea75700472534e56233964bd8d09e303a92f7196f8ae53075000dc08e432` → `344f3d79eeea562414939dd23e5a360b762a1e1478940ff2791eb9fc392e11a2`
- `10k-workers.json`: `3e46a034b18e2cce5327601d819297da72d0b10e3244349b59d90acae3069c0b` → `eecea0249e30ca09973e19c2bcad521a8e702f1f6f05130bc9483e1aafc37b2c`

## Findings

### Blocker — artifact consistency is tied to the executing checkout rather than the evidence SHA

**Location:** `internal/runner/phase3_artifacts_test.go:27-40`, `internal/runner/phase3_experiment_test.go` (`phase3Environment` / `currentGitSHA`), and `docs/perf/issue13-phase3-a9b6fb2/environment.json:18`.

**Trigger:** Run the required consistency command at exact review HEAD `1e5b5b0`.

**Observed:** regeneration calls `currentGitSHA()` and emits `1e5b5b0...`; committed evidence correctly records the code-under-test SHA `a9b6fb2...`. The byte comparison then fails `environment.json is not reproducible from samples.json`.

**Impact:** the exact review HEAD cannot satisfy its checked-in artifact gate, so a PASS verdict would contradict an executed required command. This does not invalidate the reconciled samples or correctness results, but it blocks acceptance until the test distinguishes immutable measurement provenance from the later evidence commit/checking checkout.

**Suggested direction:** make offline regeneration consume/fix the recorded measurement SHA as immutable provenance, or compare deterministic derived artifacts while separately validating that the recorded SHA is an ancestor/approved code-under-test commit. Do not simply overwrite measurement provenance with the docs commit SHA.

### Gap — 10k output is a fresh single observation, not byte-reproducible evidence

`TestPhase3Scale10K` correctly fails closed on row/output/validation/resource-cleanup invariants, but its four output files contain nondeterministic timings/resources and current HEAD metadata. A fresh valid run therefore cannot equal checked-in bytes. This is expected for measurement data, but means rerun equality is not an appropriate integrity criterion; correctness-field reconciliation and explicit provenance are the meaningful checks. The task's requested comparison found differences and this review does not treat them as a performance regression or statistically meaningful change.

## Residual risks

- The strace run and raw syscall selection were inspected as checked-in evidence but not rerun; this QA run did not invoke `strace`.
- 10k has one observation per worker count and cannot establish speedup, regression, or significance.
- No real MySQL performance or CLI subprocess performance was executed.
- This review did not run 100k/1M scale or non-Linux execution.
- The full branch diff includes earlier phase work; detailed source inspection here was bounded to phase-3 commits/evidence and relevant instrumentation/tests.

## Repository state

Only this authorized review file was created. The tracked phase-3 evidence and source files were not edited. Before writing this review, `git status --short` was empty after every required test. No files were staged, committed, pushed, or submitted to a network service.
