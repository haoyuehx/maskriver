# Issue #14 acceptance report — M2 local transactional recovery

**Issue:** [#14](https://github.com/haoyuehx/maskriver/issues/14)
**Protocol:** `m2-recovery-v1` / schema `maskriver-recovery-v1`
**Baseline `main`:** `aee0b801c738b89a6267e06c575c95878f785c7f`
**Implementation head:** `e1f2242` (code head; the 10k/100k/1M scale evidence was recorded at `09612f8`, before the `e9ae004`/`e1f2242` pre-write-gate and harness fixes, so those numbers describe the measured loop at that head, not the final head)
**Design PR:** #79 (head `86ec313`, merge `979446ac5de63d638bbe85245119c716da9af678`)
**Implementation PR:** #80 (merge `da1984c6cda7e40cfaad272d7b65522b2e108687`)
**Final docs / acceptance PR:** the PR carrying this report (merge SHA recorded in the closing comment)
**Final `main` after merge:** recorded in the closing comment and re-read from GitHub

## 1. Supported scope

Linux amd64, local SQLite, rollback journal (`journal_mode=delete`), session
`synchronous=FULL`, foreign keys ON, `query_only=OFF`. Recovery is enabled only
after an explicit `maskriver init-recovery` and only for runs started with the
recovery schema present. Replacement must be a pure deterministic function of
source + frozen plan + key; `null`, `blank`, `redact`, `format_random`,
`fake_email` and the nine `cn_*` strategies are allowlisted.

Explicitly not supported and failed closed: MySQL recovery, Windows/macOS
recovery, WAL recovery, cross-store/persistent mapping atomicity, generic
distributed or exactly-once delivery, real power-loss durability proof,
TOCTOU-proof path binding, and `--force` conflict repair.

## 2. Delivered work packages

| WP | Delivery |
|---|---|
| WP0 | [Recovery protocol](design/issue14-recovery-protocol.md): threat model, state machine, manifest/progress, same-transaction boundary, idempotency argument, commit-gap policy, PK privacy, schema upgrade, failure matrix, interface list, non-claims. |
| WP1 | `internal/db`: `maskriver-recovery-v1` DDL, explicit migration, same-transaction UPDATE + receipt + progress + manifest CAS, no-op progress, CAS/terminal-state guards, KCV, outbox-run cross-check, read-only reader, reserved names, legacy outbox compatibility. |
| WP2 | `internal/runner`: `StreamingResume` fixed gate order, recovery-aware preflight, prefix/suffix verification, bounded progress/receipt cross-check, source cursor reconstruction, multi-table/NULL continuation, commit-unknown reconciliation, strategy allowlist, failure states. |
| WP3 | `cmd/maskriver`: `init-recovery`, read-only `status`, explicit `--streaming --apply --resume --run-id`, distinct exit codes, no auto run/rename/DDL. |
| WP4 | Deterministic test-only fault seam, DB/runner/CLI fault tests, real CLI subprocess interruptions at 1/10/100/500 batches, between tables, after validation, before/after commit, A/B result equality, 10k/100k/1M scale evidence, documentation and this report. |

## 3. Transaction and idempotency proof

For a business page, one SQLite transaction contains:

```text
business UPDATE
+ outbox receipt INSERT
+ recovery progress INSERT (kind=batch)
+ manifest UPDATE ... WHERE checkpoint_sequence = expected   (must affect 1 row)
COMMIT
```

No-op pages contain the progress INSERT and manifest CAS only. A table boundary
contains a boundary progress event and, on the final table, `business_complete=1`.
Contracts/tests prove:

- any pre-commit failure rolls back all effects (`TestRecoveryCheckpointFailureRollsBackBusiness`);
- a real commit whose response is lost advances the checkpoint and receipt; a new
  process sees it and skips it (`TestRecoveryCommitUnknownAfterRealCommit`,
  `TestRecoveryResumeCrashAfterCommitBeforeResponse`);
- a commit attempt error is always `ErrCommitUnknown`, never guessed from text;
- the CAS loser gets a conflict, not a second write (`SetManifestState`/progress CAS tests);
- the target prefix must equal the deterministically recomputed replacement and
  the suffix must equal the source; divergence is `CONFLICT` and is not overwritten;
- the key-check value binds the actual key bytes, so a wrong key with a correct
  `KeyID` is refused even when no batch committed (`TestRecoveryResumeRefusesWrongKeyBeforeFirstBatch`);
- repeated resume is idempotent: receipts and values do not change.

This is **effect idempotency inside the supported local transaction**, not a
general exactly-once delivery guarantee.

## 4. Fault-injection evidence

| Scenario | Evidence | Result |
|---|---|---|
| Crash before any batch | `TestRecoveryResumeCrashBeforeCommitRollsBack`, pre-commit CLI fault | PASS, rollback, resume completes |
| Crash after UPDATE / receipt / checkpoint before commit | DB hook tests + `before_commit` CLI fault | PASS, all rolled back |
| Real commit, response lost | `TestRecoveryCommitUnknownAfterRealCommit`; CLI `after_commit` exit | PASS, no replay |
| Ambiguous commit | manifest `commit_unknown` + authoritative re-read | PASS, reconciled without retry-on-error |
| Replay committed batch | prefix verification + A/B digest | PASS, not replayed |
| Resume ×3 after completion (4 total including the first completion) | `TestRecoveryResumeSkipsCommittedBatchesAndIsIdempotent` | PASS, receipts unchanged |
| All-NULL page | `TestRecoveryResumeAllNullPages` | PASS, progress-only, no fake committed rows |
| Multi-table, between tables, second table | `TestRecoveryResumeNullPagesAndMultiTable`, CLI 1/10/100/500 | PASS |
| Crash after all business commits, before validate | CLI `after_validate`, runner test | PASS, re-validated |
| Validate failed | `TestRecoveryValidationFailedRunIsNeverComplete` | PASS, never `complete` |
| Source changed | changed-source test | PASS, refused |
| Target externally changed | tampered target + late-change test | PASS, `CONFLICT`, no overwrite |
| Manifest wrong version / gap / ahead of log | schema/version tests, `TestRecoveryResumeRejectsManifestAheadOfProgress` | PASS, refused |
| Duplicate batch/sequence, tampered progress | DB/runner tests | PASS, `CONFLICT` |
| Wrong key / KCV mismatch | wrong-key test | PASS, `ErrUnsafe` |
| Missing/partial recovery schema | schema/legacy tests | PASS, `ErrUnsupported`, no fallback |
| `SQLITE_FULL` | outbox full-disk test | PASS, fail closed |
| Context cancel | outbox/runner cancellation tests | PASS, committed prefix remains recoverable |
| Concurrent resume | progress/manifest CAS | PASS, one winner, other `CONFLICT` |
| Kill *during* the SQLite commit syscall / receipt-time ENOSPC / fsync failure | not deterministically producible | **UNVERIFIED**, inherited Issue #13 G7/G9 limits |

## 5. Real CLI interruption evidence

`cmd/maskriver/recovery_e2e_test.go` runs every command in a separate OS process
through the real `main` entry point. A test-only hook (compiled only into the test
binary) exits at a precise stage; the production binary has no environment-driven
fault path. Scenarios: after 1, 10, 100 and 500 committed batches; before commit;
after validate. Each scenario then runs read-only `status`, a separate `--resume`,
strict `validate`, and a second resume. The resumed target's ordered `id,email`
email list equals the uninterrupted baseline (the harness compares sorted masked
emails, not a single row digest); the second resume is separately checked to
leave the target bytes unchanged.

## 6. Scale evidence

[Recovery scale report](perf/issue14-09612f8/summary.md), Linux amd64, Go 1.26.3,
BatchSize 256, Workers 4, synthetic data:

| Rows | Baseline apply | Interrupted apply | Resume | Resume / baseline | Peak RSS (resume) | A/B equal | Strict validate |
| ---: | ---: | ---: | ---: | ---: | ---: | :---: | :---: |
| 10k | 1.47 s | 0.81 s | 0.86 s | 0.586 | 23.4 MiB | yes | PASS |
| 100k | 12.38 s | 7.38 s | 7.13 s | 0.576 | 33.7 MiB | yes | PASS |
| 1M | 109.68 s | 59.33 s | 67.15 s | 0.612 | 37.9 MiB | yes | PASS |

1M is a single sample on this machine; the scale test is opt-in and not run in
CI. No production throughput claim follows.

## 7. Independent review

- **Architecture review (fresh context, commit 2251823):** CHANGES REQUIRED
  (P1-1 fixed-point rule, P1-2 CAS/manifest-log agreement, P1-3 metadata order).
- **Second-context architecture recheck (commit a057fbc; later `09612f8` P2 fixes covered by the security recheck and suites):** **PASS**, no P0/P1.
  The P1 items are closed by code and tests; remaining findings are P2.
- **Security review (fresh context, commit 2251823):** CHANGES REQUIRED
  (P1: key material not bound for a vacuous prefix).
- **Second-context security recheck (commit 09612f8):** **PASS**, no P0/P1. The
  KCV fix is verified before any write, including a vacuous prefix; seven P2
  findings were dispositioned (allowlist timing, eligibility, final-ordinal
  guard, key-generation note, sidecar test, `init-recovery` precondition,
  evidence paths).
- **QA (fresh context):** **PASS**, no P0/P1. Independently ran `gofmt`, `vet`,
  build, package suites, focused DB/runner recovery tests, real CLI E2E and
  race; independently observed an interrupt→status→resume→strict-validate run
  with A/B value identity and an unchanged target on repeat resume. Details in
  `docs/reviews/issue14-qa-verification.md`.

Review files are attached by path in the PR comments:
`docs/reviews/issue14-*.md`.

## 8. AC traceability

| Original AC | Status | Evidence |
|---|---|---|
| Documented commit/checkpoint protocol states what is atomic/idempotent and what an interruption means | PASS | protocol §4–§9, this report §3 |
| Fault injection with deterministic tests | PASS | §4 matrix, DB/runner/CLI tests |
| Resume proven idempotent | PASS | repeated-resume tests, A/B hashes, 10k/100k/1M |
| Committed-range reporting matches reality | PASS | progress/receipt cross-check, `status`, conflict tests |
| No original values/keys in manifests/checkpoints | PASS | schema review, tests, no value fields |
| No double masking | PASS | prefix verification + deterministic recomputation + A/B equality |
| No silent data loss | PASS | conflict refusal, validate, row-count/key checks |
| No production database used | PASS | all fixtures are `t.TempDir()` synthetic SQLite |
| `architecture/test-plan/feature-matrix` updated with evidence and non-guarantees | PASS | this PR |
| Protocol document, fault output, resume logs, unsupported failure modes | PASS | this report + linked tests/perf files |

## 9. Remaining risks and accepted P2

- Recovery is local SQLite only; MySQL/Windows/WAL and cross-store mapping
  recovery are unsupported and fail closed.
- No proof of real power-loss durability beyond SQLite's local atomic-commit
  behaviour at `synchronous=FULL`; kill during the commit syscall, receipt-time
  ENOSPC and fsync failure remain UNVERIFIED.
- Path binding is preflight + identity recheck, not TOCTOU-proof.
- Approval validity is checked once at resume start, like Issue #13's one-shot
  apply; a multi-hour resume is not continuously re-authorized.
- Non-planned columns and balanced delete+insert pairs outside the planned
  key/value contract are not the recovery verification target.
- A `validation_failed` manifest is terminal for resume and needs operator
  investigation / a new run.
- Recovery metadata is retained in the masked database and is never auto-dropped.
- The scale test is opt-in; CI covers deterministic correctness and fault tests,
  not the 10k/100k/1M matrix.
- Accepted P2 from reviews: a rare `SQLITE_BUSY/LOCKED` path is mapped to
  `ErrConflict`; the fixed-point replacement guard is proven at unit level; the
  scale/bounded-RSS evidence is opt-in rather than a CI gate.
