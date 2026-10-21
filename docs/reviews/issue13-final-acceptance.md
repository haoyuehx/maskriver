# Issue #13 — final acceptance matrix (2026-10-21)

**Approved scope:** original Issue #13 acceptance criteria plus the 2026-10-20 S2 scope revision
(legacy O(K) retained; the new streaming path bounded). Windows/MySQL streaming, anchor/seal and #14
recovery are outside the approved scope.
**Accepted code head:** `e02fa8e595a5ff4b008a173ed8e146e75794d4b6` (PRs #76, #77 on top of #75).
**Resource decision:** `docs/design/issue13-adr08-final-proposal.md` —
`ADR-08 APPROVED — DELEGATED TECHNICAL DIRECTION (2026-10-21)`, not a GitHub Owner signature.

Every PASS cites code and/or executed evidence. A design document alone is not evidence. Items that
remain unverified are not upgraded.

## Functional

| Criterion | Status | Evidence |
|---|---|---|
| Bounded concurrency (1–16 workers, fixed queue, deterministic order) | PASS | worker pool reuse; `TestStreamingApplyWorkersAgree`; 1/2/4/8/16 identical target; queue non-zero refused |
| Cancellable | PASS | context through runner/outbox/history/worker; cancel and close tests |
| Deterministic results | PASS | identical masked values across worker counts and repeated CLI samples |
| Streaming path has no O(K) receipt memory | PASS | counters only; receipt reader pages; `TestStreamingApplyCommitsEveryBatch` |
| Row/field size bound before allocation | PASS | `db.PageWithin` same-snapshot length preflight; 8 MiB rejected with ~13 KiB total alloc vs ~33.5 MiB for legacy `Page`; lookahead/huge-key allocation tests; runner checks static/late/source/target/replacement rows |
| Column/table metadata bound | PASS | `ValidatePlan`, request validation |
| Directory bound | PASS | chunked enumeration, streaming entry/name caps, legacy compatibility test |
| Queue bound | PASS | `MaxQueue` 0-only |
| Process/disk guarantee matches documented semantics | PASS | ADR-08 approves `MaxProcessBytes` as admission-only and `MaxDiskBytes` as admission/post-commit; not advertised as OS RSS cap or quota |
| Sample/validate resource boundary | PASS | Sample 1 MiB/value + 16 MiB/column fail-closed; streaming validation through `PageWithin`; legacy Validate unchanged |

## Security

| Criterion | Status | Evidence |
|---|---|---|
| Real Ed25519 M2 approval | PASS | separate signed record; trust/expiry/revocation; CLI E2E |
| Canonical plan binding | PASS | content digest covers Scope/NormalizationVersion/type/PK/order; drift tests |
| Unauthorized write refused | PASS | no-apply, no-approval, legacy approval, revoked, missing/wrong schema E2E |
| No raw values or keys leaked | PASS | CLI E2E secret scan; sanitized summaries/errors; independent Security reviews |
| CommitUnknown not retried | PASS | WP2/runner tests; CLI prints `retry=never` |
| Transaction correctness in supported env | PASS | same-transaction receipt tests; independent Validate |
| Unsafe target directory refused | PASS | `TestE2EStreamingRefusesWorldWritableTargetDirectory` |
| Oversized row cannot bypass budget | PASS | source/target/static/late/lookahead/huge-key/replacement tests; no fallback to `Page` |

## Testing

| Criterion | Status | Evidence |
|---|---|---|
| Race | PASS | `go test -race ./internal/db ./internal/runner ./internal/history ./cmd/maskriver` |
| Fuzz | PASS (bounded run) | `FuzzStreamingRequestValidate`, ~1.8M execs / 5 s |
| Leak (connections/goroutines) | PASS (bounded scope) | outbox close/rollback tests, pool stats, goroutine checks; no long-running leak detector |
| Cancellation | PASS | cancel tests and propagation |
| Fault injection | PASS / UNVERIFIED | UPDATE/receipt failure, full disk, commit pre/post exit, path/WAL/schema; receipt-time ENOSPC/fsync and exact mid-commit kill UNVERIFIED |
| Worker result consistency | PASS | runner equivalence test |
| Legacy M1 regression | PASS | full suite + CLI E2E; defaults unchanged |
| Large-field pre-materialisation rejection | PASS | DB and runner opt-in evidence; independent QA reproduced |

## Performance

| Criterion | Status | Evidence |
|---|---|---|
| Runner/CLI real SQLite | PASS | WP3 + WP3-G evidence |
| 10k / 100k / 1M | PASS | `docs/perf/issue13-wp3g-e02fa8e/` (27 samples; 3 per cell) |
| Repetition and raw data | PASS | CSV + environment.json; medians generated from raw rows |
| RSS/Heap observation | PASS (with limits) | CLI VmHWM ~24/38/39.5 MiB; child heap unavailable; DB test measures process allocation |
| Method and limitations | PASS | environment.json + summary.md; no p95 claim from 3 samples |
| Performance delta disclosed | PASS | apply throughput ~15–28% lower at 1M than WP3-F, consistent with the extra length preflight and replacement check |

## Configuration and documentation

| Criterion | Status | Evidence |
|---|---|---|
| ADR-08 values and semantics approved | PASS | delegated technical direction, explicit guarantee levels |
| Feature Matrix truthful | PASS | updated to WP3-G status and limits |
| Legacy M1 O(K) retained | PASS | S2 revision; docs and code unchanged |
| Streaming limits explicit | PASS | ADR-08 + enforcement audit |
| MySQL/Windows not claimed | PASS | documented unsupported/unverified |
| Anchor/checkpoint/exactly-once out of scope | PASS | not implemented, not required |

## G5 / G7 / G9 disposition

| Risk | Current supported range | Evidence | Unverified | Required by approved #13 scope? | Accepted |
|---|---|---|---|---|---|
| G5 path identity / TOCTOU | trusted local isolated directory; world-writable target refused; path/inode/symlink checks in outbox open | `TestE2EStreamingRefusesWorldWritableTargetDirectory`; WP2 identity tests | file-handle TOCTOU against an attacker who can replace the directory | No (threat boundary excludes active local attacker) | Yes, bounded risk accepted in ADR-08 |
| G7 receipt-time ENOSPC/fsync | UPDATE-stage SQLITE_FULL and receipt INSERT failure roll back atomically; CommitUnknown never retried | WP2/runner transaction tests | exact ENOSPC/fsync during receipt commit | No (atomicity already tested; no durability claim) | Yes, UNVERIFIED disclosed |
| G9 mid-commit kill | exit before/after commit yields no-commit / CommitUnknown, never retried | outbox commit tests | exact kill inside commit | No (#14 recovery is out of scope) | Yes, UNVERIFIED disclosed |

## Final verdict

All required functional, security, testing, performance and documentation criteria are satisfied for
the approved scope, with the stated guarantee levels and unverified items disclosed. No open P0/P1.
**READY FOR CLOSURE.**
