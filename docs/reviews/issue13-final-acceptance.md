# Issue #13 — final acceptance matrix (WP3-F)

**Technical head:** `41e96b18130d9becfabb15d2bca502054331c1cd`
**Approved scope:** original Issue #13 acceptance criteria plus the 2026-10-20 S2 scope revision
(legacy O(K) retained; streaming path bounded). Windows/MySQL streaming, anchor/seal and #14
recovery are outside the approved scope.

Every PASS below points at code and/or executed evidence. A design document alone is not evidence.

## Functional

| Criterion | Status | Evidence |
|---|---|---|
| Bounded concurrency (1–16 workers, fixed queue, deterministic order) | PASS | worker pool reuse; `TestStreamingApplyWorkersAgree`; 1/2/4/8/16 identical target |
| Cancellable | PASS | context through runner/outbox/history/worker; cancel and close tests |
| Deterministic results | PASS | identical masked values across worker counts and repeated CLI samples |
| Streaming path has no O(K) receipt memory | PASS | counters only; receipt reader pages; `TestStreamingApplyCommitsEveryBatch` reads pages |
| Row/field size bound | **PARTIAL** | post-materialisation `pagePayloadBytes` fails closed before writes; `TestStreamingRowBytesPostMaterializationEvidence` shows 8 MiB already allocated before the check |
| Column/table metadata bound | PASS | `ValidatePlan`, request validation |
| Directory bound | PASS | chunked enumeration, streaming entry/name caps, legacy compatibility test |
| Queue bound | PASS | `MaxQueue` 0-only enforcement |
| Process/disk guarantee | **PARTIAL** | admission + post-commit checks; not OS RSS quota or filesystem quota |

## Security

| Criterion | Status | Evidence |
|---|---|---|
| Real Ed25519 M2 approval | PASS | separate signed record; trust/expiry/revocation; CLI E2E |
| Canonical plan binding | PASS | content digest covers Scope/NormalizationVersion/type/PK/order; drift tests |
| Unauthorized write refused | PASS | no-apply, no-approval, legacy approval, revoked, missing/wrong schema E2E |
| No raw values or keys leaked | PASS | CLI E2E secret scan; sanitized summaries/errors |
| CommitUnknown not retried | PASS | WP2/runner tests; CLI prints `retry=never` |
| Transaction correctness in supported env | PASS | same-transaction receipt tests; independent Validate |
| Unsafe target directory refused | PASS | `TestE2EStreamingRefusesWorldWritableTargetDirectory` |

## Testing

| Criterion | Status | Evidence |
|---|---|---|
| Race | PASS | `go test -race ./...` (history/runner/cmd in WP3-F run) |
| Fuzz | PASS (bounded run) | `FuzzStreamingRequestValidate`, 10 s, 3.3M execs |
| Leak (connections/goroutines) | PARTIAL | outbox connection-close tests and goroutine counts; no long-running leak detector |
| Cancellation | PASS | cancel tests and cancellation propagation |
| Fault injection | PASS / PARTIAL | UPDATE/receipt failure, full disk, commit injection, path/WAL/schema; mid-commit kill and receipt-time ENOSPC UNVERIFIED |
| Worker result consistency | PASS | runner equivalence test |
| Legacy M1 regression | PASS | full existing suite + CLI E2E; defaults unchanged |

## Performance

| Criterion | Status | Evidence |
|---|---|---|
| Runner-level 10k/100k/1M | PASS | WP3 evidence; heap flat 1–4 MiB across 2–3,907 batches |
| Real CLI subprocess 10k/100k/1M | PASS | `docs/perf/issue13-wp3f-41e96b1/` (33 samples) |
| Repetition | PASS | 10k ×5; 100k ×3; 1M ×3 per configuration |
| Raw results and environment | PASS | CSV + environment.json |
| RSS/Heap observation | PASS (with limits) | CLI VmHWM ~24/38/39 MiB; child heap not available; runner heap recorded separately |
| Complete measurement method | PASS | method and limitations documented |
| Unmeasurable items listed | PASS | RSS sampling limits, child heap, MySQL/Windows, durability |

## Documentation

| Criterion | Status | Evidence |
|---|---|---|
| Feature Matrix truthful | PASS | updated to WP3-F status and limits |
| Legacy M1 O(K) retained | PASS | S2 revision; docs and code unchanged |
| Streaming limits explicit | PASS | audit + ADR proposal + perf summary |
| Windows/MySQL not claimed | PASS | documented unsupported/unverified |
| Anchor/checkpoint/exactly-once out of scope | PASS | not implemented, not required |

## Decision inputs

- **ADR-08:** `PENDING APPROVAL`; MaxRowBytes is post-materialisation, MaxProcessBytes is admission-only.
- **Blocking technical gap:** a byte-aware page read in `internal/db` is required before MaxRowBytes can
  be called a pre-allocation guarantee. `internal/db` was read-only in this round; the proposed DB
  sub-task is not authorized or implemented.
- **Unverified, non-blocking for the approved scope:** mid-commit kill, receipt-time ENOSPC/fsync,
  file-handle TOCTOU, MySQL/Windows streaming.

## Verdict

**ISSUE #13 BLOCKED** — the technical work, CLI scale evidence and reviews can complete, but the
approved row-size guarantee cannot be enforced before allocation with the current DB read API, and
ADR-08 is not approved. This matrix does not justify closure.
