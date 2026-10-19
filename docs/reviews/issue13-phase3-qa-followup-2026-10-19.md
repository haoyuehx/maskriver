# Issue #13 Phase-3 Independent QA Follow-up — 2026-10-19

## Verdict

**PASS for the requested local QA gate at exact HEAD `c49b9ceee75d4cc6c8e56f32c23ef9d4740e4186`.**

All required commands passed. No blocker was found in artifact consistency, observer-overhead evidence, mutation guards, receipt/row/validation/closure invariants, or the fresh 10,000-row SQLite run.

Remote CI/PR does not exist and was not claimed. MySQL performance, Windows runtime, and strace reproduction remain unverified.

## Environment and isolation

- Worktree: `/home/haoyue/Project/worktrees/maskriver/issue13-phase3-review-c49b`
- Detached HEAD: `c49b9ceee75d4cc6c8e56f32c23ef9d4740e4186`
- Git common dir: `/home/haoyue/Project/maskriver/.git`
- Platform: Linux/amd64
- Go: `go1.26.3`
- CGO: enabled
- Status before: clean, detached HEAD
- Status after: clean, detached HEAD
- Staged files: none
- Repository writes: none
- Authorized external output only: `/tmp/maskriver-phase3-final-qa-10k/`

## Required command results

| Command | Result |
|---|---|
| `go test -count=1 ./...` | PASS; all packages passed, runner 27.045s |
| `go test -race -count=1 ./...` | PASS; all packages passed under race, runner 32.790s |
| `go vet ./...` | PASS; no diagnostics |
| `MASK_RIVER_PHASE3_DIR=$PWD/docs/perf/issue13-phase3-c747538 go test -run '^TestPhase3ArtifactConsistency$' -count=1 ./internal/runner/` | PASS |
| `MASK_RIVER_PHASE3_OVERHEAD_DIR=$PWD/docs/perf/issue13-phase3-c747538/observer-overhead go test -run '^TestPhase3ObserverOverheadArtifactConsistency$' -count=1 ./internal/runner/` | PASS |
| `MASK_RIVER_PHASE3_10K_DIR=/tmp/maskriver-phase3-final-qa-10k go test -run '^TestPhase3Scale10K$' -count=1 ./internal/runner/` | PASS in 10.808s |

## Independent invariant reconciliation

Committed 256-row evidence:

- 30 samples: exactly 10 each for BatchSize 16, 64, and 256.
- Exactly 210 receipts and successful commits.
- Exactly 7,680 committed rows, receipt rows, and UPDATE statements.
- Every trial was complete and independently validated.
- Begin/commit/receipt counts matched `ceil(256 / BatchSize)`.
- No begin failure, commit error/unknown, or rollback attempt was recorded.
- Every batch’s receipt rows equaled its UPDATE count.
- Batch statement counts equaled `updates + other statements + BEGIN/COMMIT`.
- Reader/writer open connections after close were exactly `0/0`.
- Transaction and batch CSVs contained exactly 30 and 210 records.

Fresh 10k run, workers `1/2/4/8/16`:

- Each run committed exactly 10,000 rows in 157 receipts.
- `complete=true`, `validate_passed=true`, and `source_unchanged=true`.
- Target row count was exactly 10,000.
- Reader/writer connections after close were exactly `0/0`.
- The passing test also compared complete per-primary-key outputs against workers=1.
- Observed workflow throughput was 5,095–5,267 rows/s; this is a single descriptive observation per worker count, not a speedup or significance claim.

## Reviewer-fix verification

1. **Historical provenance/artifact gate:** verified fixed. The exact-head artifact command passed even though committed measurement provenance remains SHA `c747538…` while the checkout is `c49b9ce…`. `TestPhase3EnvironmentValidationUsesMeasurementProvenance` also passed.
2. **Throughput and negative timing validation:** verified fixed. Explicit mutation tests passed:
   - inconsistent derived throughput rejected;
   - negative driver commit timing rejected;
   - negative per-batch other-SQL timing rejected;
   - negative transaction-window timing rejected.
3. **Observer-copy bias mitigation:** verified in code and artifacts. SQLSpy exposes constant-time count cursors and suffix-only `StatementsSince`/`EventsSince`; diagnostic transactions use those cursors. Observer artifacts regenerated exactly and contain 60 valid observations forming 30 complete plain/instrumented pairs, 10 per batch size. This quantifies observer perturbation; it does not claim to eliminate it.

The post-marker independent Validate claim is present in the strace test/evidence, but strace itself was not rerun during this QA.

## Safety and residual risks

- Evidence scan found no private-key material, DSN, credential/password markers, or production data.
- Fixtures use synthetic `@example.invalid` data.
- No network, push, comment, Issue mutation, commit, or staging occurred.
- `/tmp/maskriver-phase3-final-qa-10k/` remains as the authorized fresh-run output.
- Real MySQL performance/CLI E2E, Windows runtime, non-Linux resource behavior, 100k/1M scale, and fresh strace execution remain unverified.
- Remote final-head CI is unavailable because no remote PR/CI exists.