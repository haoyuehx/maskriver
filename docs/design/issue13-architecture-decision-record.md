# Issue #13 Architecture Decision Record — bounded Apply receipts and audit consistency

**Status:** PROPOSED; all owner decisions below remain `PENDING OWNER DECISION`. This ADR is not authorization to change production behavior or frozen contracts.

**Current PR context at drafting:** PR #71, branch `design/issue13-bounded-receipts-20261020`. Baseline `main=9d6e03627c15fa31df051b16370f79e895e96bcc`. Independent Architecture and Security reviews examined head `e1faa582ca5fa2d07478f081418c968101049f97`; their findings/limits are recorded in `docs/reviews/`. This revision separates runner execution states from verifier outcomes to correct the Architecture P2 and must itself be reviewed at its new exact SHA.

**Repository facts at `e1faa58`:** PR #70 is merged; Issue #13 is OPEN; #14 is not authorized; `pkg/contracts.APIRevision=m1a-v1` is frozen; `v0.1.0-dev` is unchanged. Main verified Ubuntu, Windows and isolated MySQL CI pass on PR #71 head `e1faa582ca5fa2d07478f081418c968101049f97`; these documentation checks do not validate a later head or any outbox implementation.

## Purpose and invariant

Decide what evidence would be needed to satisfy Issue #13's original memory AC without losing receipts, masking partial commits, weakening approval, or pretending that separate durable stores form one transaction. This document distinguishes a proposal from owner approval and from implementation evidence.

**Acceptance provenance:** Main fetched the live Issue #13 body on 2026-10-20 (`https://github.com/haoyuehx/maskriver/issues/13`, active section dated 2026-10-19; older backlog text is explicitly superseded there). The verbatim acceptance checklist is:

- Function: “Concurrency is bounded and cancellable; ordering/partitioning is deterministic; memory growth is bounded by batch × workers.”
- Tests: “go test -race clean; fuzz targets run; leak checks for goroutines and connections; failure injection under cancellation.”
- Security: “Synthetic data only; no original values or keys in benchmark output or profiles; no claim about PII handling derived from performance runs.”
- Docs/Feature Matrix: “method and measured results recorded with the exact environment; Feature Matrix rows updated only for what was measured.”
- Final evidence: “benchmark command, environment, repeated-run distribution, correctness evidence under concurrency, and the explicit list of what was not measured.”

The live Issue also asks for a synthetic real-SQLite Scan→Preview→independent Approve→Apply→Validate workflow, 128/512/2048 CI and progressively larger 10k/100k/1M local scale where resources permit, honest metrics/environment, and states MySQL adapter evidence is not MySQL CLI E2E. This citation is the live Issue source; independent reviewers could not fetch GitHub themselves.

Original AC wording remains authoritative. This ADR does **not** reinterpret it to mean “only the new API” or “only Go heap” without an explicit Issue Owner decision. No design decision here changes `m1a-v1`, Apply transaction policy, CLI authorization, BatchSize defaults, release, or #14 scope.

## Current memory and receipt facts

- Legacy `Apply` returns every confirmed batch in `ApplyReport.Committed []BatchReceipt`. Its backing array is Θ(K), where K is successful non-empty committed pages; for fixed table count and dense writable rows, worst-case K=Θ(N/B). Current CLI also materializes this report, then iterates it.
- The runner processes one keyset page/transaction at a time and has a bounded worker pool, but that does not bound the returned receipt slice.
- Approval lookup currently calls `os.ReadDir` over the complete history directory to preserve duplicate-plan ambiguity detection, so an Apply can allocate O(H + filename bytes) even at fixed N.
- Plans/scopes/schema metadata, row payload, page lookahead, verifier issue locations, DB driver buffers, SQLite/MySQL caches, Go GC/runtime and OS file cache are separate resource terms. Phase-5 measured receipt capacities are evidence for its fixture only, not proof of a strict bound.
- Existing phase-5 resource sampler itself appends one record per 100 ms and copies the collected slice at stop; it is test instrumentation, not product memory evidence for arbitrarily long runs.

## Formal decision table

| ID | Decision | Alternatives | Recommendation (not approval) | Evidence / risk | State |
|---|---|---|---|---|---|
| ADR-01 | Does strict memory AC apply to all Apply APIs or only a new streaming path? | S1 all APIs; S2 new streaming path only; S3 bounded legacy maximum | S1 is the literal safest reading; S2 is the only compatibility-friendly path but changes AC scope; S3 is a cap, not an asymptotic solution. Require Owner to select; no silent narrowing. | Frozen return DTO proves legacy Ω(K). No data proving Issue permits API-specific AC. | **PENDING OWNER DECISION** |
| ADR-02 | May legacy full-receipt API retain O(K)? | No; yes as compatibility API; cap/retire later | Recommend retain exact `m1a-v1` semantics, label O(K), and do not call global AC complete while it is in scope. | Existing callers can observe every element. Changing semantics is breaking. | **PENDING OWNER DECISION** |
| ADR-03 | Required data-commit/audit guarantee? | A1 best effort; A2 durable reconciliation; A3 same-transaction audit row plus independent witness | Recommend A3 for each batch's receipt, independent run enrollment and per-batch anchor acknowledgments, plus terminal seal. If same-target DB atomicity cannot be proved, stop rather than downgrade to A1. | DB transaction and external anchor are separate failure domains; anchor outage leaves committed data incomplete. | **PENDING OWNER DECISION** |
| ADR-04 | May target DB schema include a private audit outbox? | No schema changes; pre-provisioned namespaced tables; separate store | Recommend pre-provisioned reserved tables in the same target database, with explicit migration/validation and no silent runtime DDL. | SQLite DML in one DB transaction can be atomic; MySQL requires same primary database and transactional engine (e.g. InnoDB) for both tables. Current code does not prove/implement this. | **PENDING OWNER DECISION** |
| ADR-05 | May an independent durable audit anchor be required? | No; local only; external append-only signer/ledger | Recommend external trust anchor for claims of tamper-evident completed runs; without it, claim only accidental-corruption detection and local consistency. | No such service exists in current product; key custody, availability, retention and threat model need owner/infrastructure approval. | **PENDING OWNER DECISION** |
| ADR-06 | Are third-party receipt Consumers allowed? | Arbitrary callback; trusted internal sinks only; isolated process | Recommend no user-supplied callback in first streaming API. Use an internal synchronous transactional outbox writer. | A Go callback can ignore ctx, block forever, panic or re-enter. Bounded queue does not bound its execution. | **PENDING OWNER DECISION** |
| ADR-07 | Which platforms/storage must provide durable semantics? | Linux only; Linux+Windows; SQLite local only; MySQL too | Recommend initially support only explicitly tested local SQLite rollback-journal with required sync mode; add MySQL only after proving same-DB transactional engine and durability settings. Other filesystems/platforms fail closed/UNSUPPORTED. | Prior CI proves builds/tests, not atomic/durable outbox semantics. Network filesystems and power-loss guarantees vary. | **PENDING OWNER DECISION** |
| ADR-08 | Numeric memory/disk/metadata limits? | Define L/T/C/H/D caps; symbolic working-set claim only | Recommend symbolic budgets first, numeric values only after representative payload measurements and Owner approval; no invented “approved” cap. | Without per-row byte, plan, history and metadata limits no byte bound. Disk receipts remain Θ(K). | **PENDING OWNER DECISION** |
| ADR-09 | Legacy Plan/approval/API revision compatibility? | Global revision bump; additive parallel version; break old approval | Recommend preserve `m1a-v1` and Plan.ID/approval behavior; define additive versioned streaming request/approval binding that coexists and does not reinterpret m1a plans. | Global `APIRevision` change is included in plan identity and `ValidateShape`; old approvals can stop validating. Exact DTO remains for a separate contract proposal. | **PENDING OWNER DECISION** |
| ADR-10 | Which production modules may be changed next? | None; contracts only; runner+DB; CLI/history cross-module | No production changes now. If approved, authorize files/modules separately and coordinate owners; do not imply #14. | Current task authorizes design/docs/review only. | **PENDING OWNER DECISION** |
| ADR-11 | Which attacker/threat coverage must audit completion provide? | Local corruption; known-run tampering after trusted enrollment; pre-enrollment/whole-run deletion; host/runner compromise | Minimum honest guarantee: detect alteration only after independent anchor enrollment, for runs discoverable from that independent catalog; explicitly exclude pre-enrollment rewrite and host/runner compromise. Stronger coverage requires independent admission/attestation. | Security Review on `c44a7c3` found first-anchor authenticity and unknown-run discovery underspecified. No anchor service exists. | **PENDING OWNER DECISION** |

## S1/S2/S3 — strict memory AC scope

### S1 — every Apply API, including legacy

Legacy callers require `ApplyReport.Committed` with all K receipts when the call returns. The concrete non-zero-sized slice must retain K elements; for fixed B and dense writable data, that is Ω(N/B). Keeping `m1a-v1` result semantics and satisfying a strict N-independent result-memory bound are incompatible. Achieving S1 would require breaking/retiring/changing the legacy result contract or redefining “returned results” as an external cursor, which is a contract change. `m1a-v1` is frozen, so S1 is **not implementable under the current authorization**. A cap only makes memory bounded by a fixed maximum, not “BatchSize × Workers”, and does not preserve unrestricted large-dataset intent.

### S2 — only a new streaming path

A new API can emit one receipt synchronously or use Q=0/1 queue and return a compact summary. Legacy API remains available and O(K); existing callers still receive all receipts. CLI must opt into the new path explicitly; no silent fallback to the legacy path if the sink fails. The streaming path can claim only a specifically scoped memory guarantee after its metadata, approval scan, row bytes and runtime scope are bounded. S2 is a plausible implementation boundary but **changes the acceptance scope** if Issue #13 presently means all Apply. Only Issue Owner can approve that interpretation.

### S3 — cap legacy receipt count

A hard maximum Kmax can reject a run before opening the writer if a safe upper bound is known. To avoid partial work, it must precompute a conservative maximum `Σᵢ ceil(Nᵢ/B)` and fail before any write if it exceeds Kmax; all-NULL rows make this conservative. This caps the maximum, but receipt memory is still O(K) up to Kmax and is not a streaming/asymptotic solution. A late runtime cap after some commits is unacceptable unless it reports partial commit, but then does not protect against partial work. S3 is only a compatibility/resource guard and cannot alone pass the original AC.

**Recommendation:** evaluate S2 only after explicit Owner scope decision; retain legacy behavior. No option is approved by this ADR.

## A1/A2/A3 — audit consistency

| Model | Guarantee | Failure consequence | Decision assessment |
|---|---|---|---|
| A1 Best effort | Commit data, then append receipt to independent sink | Post-commit sink error/crash leaves committed data with missing durable receipt. Must report `AUDIT_INCOMPLETE`; cannot claim complete audit. | Does not satisfy complete audit requirement; not recommended and cannot be used to close P1 absent Owner-approved weaker guarantee. |
| A2 Durable reconciliation | A durable attempt record plus stable transaction/batch ID lets recovery inspect authoritative database-side state; unknown remains unknown when authoritative DB unavailable | Supports later classification without replay. If data and audit domains are separate, a crash can leave prepared/committed disagreement; reconciliation is not exactly-once and needs protocol and authoritative reads. | Candidate only if Owner accepts reconciliation as a future recovery surface; avoid expanding into checkpoint/resume/#14. |
| A3 Transactionally atomic batch audit | Data UPDATEs and a receipt/attempt-state transition are written in the same target DB transaction | Atomicity means both effects commit or neither, assuming one DB transaction, transactional tables and configured durability. CommitUnknown remains unknown to the caller until authoritative read. | Recommended candidate for per-batch receipt correctness. Not yet implemented/proven on SQLite/MySQL. |

A3 constraints:

1. Receipt/outbox must be a table in the exact target database/transaction. A sidecar DB, separate SQLite file, local append file, remote service, or independent `fsync` is not atomic with business writes.
2. SQLite: same `main` database and same `sql.Tx`; only supported rollback-journal modes (`DELETE`/`TRUNCATE`/`PERSIST`), verified synchronous/durability setting, local filesystem with tested locking/fsync semantics. WAL/memory/off/unknown mode is UNSUPPORTED until tested/proven for the intended contract. Current code only has a source quiescence probe; target durability settings are not currently verified.
3. MySQL: same database/schema and same primary transaction; all business tables and outbox must use transactional engine (require verified InnoDB); verify server durability/commit settings. Cross-database, nontransactional table, replica/lagging read, or adapter-only evidence is not accepted. Current product has no MySQL CLI E2E; MySQL A3 remains UNVERIFIED.
4. Outbox schema must be pre-provisioned and versioned before Apply; no DDL during Apply. It must be excluded from business scopes but included in explicit audit preflight. Schema migration/rollback/backup/retention must be approved. New target-only table is a target schema change and must not be hidden from validation/operator reporting.
5. Disk exhaustion within the same transaction must abort the whole batch or yield CommitUnknown; no business row may commit without its outbox record. If commit returns unknown, do not retry. Query same authoritative primary after recovery; matching committed record resolves the batch committed; an authoritative prepared/absent state after recovery can resolve aborted only if DB guarantees transaction recovery and no tampering. If query unavailable or evidence inconsistent, retain UNKNOWN.
6. Same-DB atomicity protects against process crash between business commit and receipt. It does **not** by itself protect against a privileged actor rewriting the DB file/tables.

## Proposed audit protocol (not implemented)

### Canonical envelope and event records

All serialization must be versioned canonical encoding (e.g. canonical CBOR with fixed schema or strictly specified canonical JSON; exact format is contract proposal). Never persist raw source values, replacements, primary keys, masking keys, DSNs, or approval private keys.

`RunEnvelope` fields: `protocol_version`, random 128-bit `run_id`, `plan_id`, `plan_revision`, target-database identity digest (not path/DSN), ordered table/scope digest, strategy/version/key IDs (opaque IDs only), explicit approval record digest/ID, audit policy ID, start timestamp, initial sequence, and state `STARTED`. Bind envelope to exact approved Plan and audit protocol. Scope/table identifiers can themselves be sensitive metadata; access control and publication must be reviewed.

Use an **append-only event model only**; no PREPARED→COMMITTED row update. Every event has distinct `event_id`, `run_id`, monotonic `seq`, `event_type`, `event_role`, canonical payload, `prev_hash`, `event_hash`; batch events also have `batch_id`. `UNIQUE(run_id,seq)` and `UNIQUE(run_id,event_id)` apply to all events; batch rows use role `PREPARE` or `BATCH_TERMINAL`, with `UNIQUE(run_id,batch_id,event_role)`. Thus one batch can have one prepare and at most one terminal outcome total (`BatchCommitted` or `BatchAborted`), never both. `BatchCommitted` references the prepare event ID/hash; batch ID is a grouping/correlation identity, not event identity or a resume/idempotency guarantee. A run has at most one terminal seal, separately constrained. Event sequences are serialized by one run owner; a duplicate/sequence collision fails closed. Backend-specific DDL must implement equivalent constraints; verifier independently rejects illegal transitions.

`BatchPrepare` is durably inserted before the data tx: `(run_id, seq, event_id, batch_id, table_ref, table_ordinal, row_count_expected, prev_hash, event_hash)`. It is an attempt event and not evidence of data commit. The external anchor registers/acknowledges the expected prepare before opening the data transaction.

In the data transaction, execute target UPDATEs and **insert a separate `BatchCommitted` event** with actual affected row count and chain hash. Presence of this event is atomic with business rows. A known rollback may append a separate `BatchAborted` event later; if that fails, the prepare remains and run is incomplete/unknown, not success. SQLite file access cannot enforce MySQL-style table grants; triggers/insert-only code are defense in depth, not protection from a privileged DB-file writer. MySQL needs least-privilege INSERT/SELECT grants with UPDATE/DELETE/DDL denied. Exact DDL, migration and backend enforcement belong to a later reviewed contract/schema proposal.

`CommitUnknown` after the underlying commit attempt: immediately return/record volatile unknown and stop. Never write a misleading ABORTED or COMMITTED event outside the data transaction. If the atomic event is visible on the authoritative recovered DB, it proves the transaction committed; if not safely observable, retain unknown. No data retry/replay is implied.

`RunSeal`: after all planned pages complete, every per-batch anchor acknowledgment exists, and final counts match, append a `COMPLETE` or `INCOMPLETE` terminal event containing final sequence, counts by status, final chain hash, plan/target/audit-policy binding, algorithm, and seal time. A crash before seal means run is **UNSEALED/INCOMPLETE**, even if all batches appear committed. An error path attempts an INCOMPLETE seal only if it can do so without changing business data; failure to seal remains detectable as no seal. A seal never upgrades an unknown batch to confirmed.

### Hash chain, signature and trust anchor

For each event define `hash_i = SHA-256(domain || canonical(event_without_hash_or_signature) || hash_(i-1))`; check canonical encoding, protocol/domain separation, exact sequence, previous-hash match, run ID, Plan ID, and record types. Hash chaining detects accidental corruption and internal discontinuity, **not** malicious rewrite of the entire log.

A trusted external append-only Audit Anchor is required for any malicious-tampering claim. Minimum candidate protocol: before business writes, register `(run_id, plan_id, target_digest, approval_digest, protocol, start_nonce)` in an independently enumerable anchor catalog; enforce unique RunID and owner/target/time partition. For each batch, prepare its expected canonical event/hash with the anchor, commit business UPDATE + DB outbox event in the same DB transaction, then have the authenticated trusted Runner attest the Commit result/event hash to the anchor before beginning another batch. Anchor stores a monotonic per-run sequence and idempotent same-event response; conflicting events fail closed. Anchor acknowledgment is not atomic with DB Commit: crash/outage after DB commit but before acknowledgment leaves the run incomplete/unknown; do not reconstruct or upgrade the missing attestation from mutable DB data after restart. No next business batch starts until current anchor event is acknowledged. Terminal seal is anchored only after all event acknowledgments and count checks.

The anchor offers signed durable receipts, authenticated lookup and independent catalog enumeration; its key is separate from reviewer/masking keys. The trusted Runner workload identity is authenticated independently; the anchor signature authenticates anchor responses, not DB truth by itself. This model trusts the authorized Runner to attest its observed successful commit and assumes attacker cannot compromise that Runner identity/process or anchor. It detects tampering of registered audit history for catalogued runs **after enrollment/anchoring**; it does not prove events before first trusted enrollment, protect against host-root/Runner compromise, or detect a run never enrolled. A deleted local run is discoverable only by comparing against the independent catalog. If Owner requires pre-registration or untrusted-host protection, a stronger independent admission/attestation protocol is necessary; absent that, do not claim those guarantees. Anchor outage means `ANCHOR_PENDING/AUDIT_INCOMPLETE`, never successful completion.

A run seal/per-batch anchor acknowledgment is not one atomic transaction with the target DB. A3 guarantees each batch's DB outbox event atomically; independent anchor registers the run before writes and witnesses each batch before the next starts. Missing enrollment, any per-batch anchor acknowledgment, or terminal seal means incomplete, never success. After process loss, do not reconstruct missing anchor acknowledgment from mutable DB rows; run remains incomplete/unknown. Repeating an identical authenticated anchor request while trusted process retains its observed Commit result is idempotent audit finalization, not business replay. This provides post-enrollment tamper evidence only under the trusted-runner/anchor boundary. Owner must approve.

### Verification outcomes

Verifier returns exactly one outcome using this precedence:

1. `UNKNOWN`: authoritative DB/anchor/catalog unavailable, or an unanchored DB event exists after process restart and cannot be trusted; never promote mutable DB evidence alone.
2. `TAMPERED_OR_UNTRUSTED`: canonical/hash/signature/key failure or a conflicting anchored digest.
3. `ORDER_OR_STATE_INVALID`: duplicate/out-of-order events or illegal transitions, including both `BatchCommitted` and `BatchAborted` for one batch, cross-run records, or a terminal event without prepare.
4. `TRUNCATED_OR_MISSING`: independent catalog/anchor proves an event, completed seal, or run existed but required local record is missing; use this rather than INCOMPLETE when trusted external evidence expects the missing data.
5. `COMPLETE_VALID`: only when terminal seal outcome is exactly `COMPLETE`; canonical envelope; run enrolled in independently enumerable trusted catalog before writes; contiguous local and anchored event states; each COMMITTED outbox event matches trusted-runner attestation and unique batch ID/rows; expected final sequence/counts; valid terminal COMPLETE seal and signed anchor receipt. This certifies only registered-run history within the trusted-runner/anchor boundary, not pre-enrollment history or host/runner integrity.
6. `INCOMPLETE_VALID`: otherwise-valid anchored prefix whose terminal outcome is explicitly `INCOMPLETE`, or a run has no terminal seal and the independent catalog has no later event/seal expectation. It never means complete; a local DB-only unanchored event instead yields UNKNOWN.

An unsigned/unanchored database dump cannot establish absence of deletion by a privileged actor. Restoring both DB and its local records to an older backup is detectable only if external anchor retention is independent and monotonic. If the external anchor is also rolled back/compromised, detection is not guaranteed. Never reuse approval private key or trust signer identity for audit seal.

## P1-1 state machine and failure table

States: `PREPARED` (attempt event durable, no business commit claim); `COMMIT_ATTEMPTED` (volatile process state only); `COMMIT_CONFIRMED` (Commit returned success and same-transaction receipt is visible); `COMMIT_UNKNOWN` (underlying Commit attempted and returned ambiguous error); `RECEIPT_DURABLE` (DB transaction and its outbox row satisfy configured durability policy); `ANCHOR_CONFIRMED` (independent anchor durably acknowledged this event); `AUDIT_INCOMPLETE` (missing enrollment/event ack/seal or anchor failure); `FAILED_BEFORE_COMMIT` (known pre-commit error and rollback known). `COMMIT_CONFIRMED`, `RECEIPT_DURABLE`, and `ANCHOR_CONFIRMED` are distinct states.

Runner execution states and verifier outcomes are separate columns. Verifier outcomes use the single-category precedence defined in §“Verification outcomes”; they are assessed only after the execution has stopped and persistent evidence is queried. `N/A` means there is not yet a stopped run to verify, not an additional verifier status.

| Injection point | Data state | Durable event/catalog state | Runner execution/report state | Verifier outcome after stop/readback | Further writes/retry |
|---|---|---|---|---|---|
| Before first `BatchPrepare` after run enrollment | No current-batch data | Registered run/envelope; no batch event | `FAILED_BEFORE_COMMIT` | `INCOMPLETE_VALID` | Stop; no retry in same run |
| `BatchPrepare` write fails/ENOSPC | No data tx started | Registered run/envelope; no prepare event | `FAILED_BEFORE_COMMIT` | `INCOMPLETE_VALID` | Fail closed before first update; do not fallback to memory |
| After anchored `BatchPrepare`, before data Commit; rollback known | No batch data | Prepare event is locally and externally anchored | `FAILED_BEFORE_COMMIT` | `INCOMPLETE_VALID` | Stop; never reapply automatically |
| Underlying Commit attempt in progress | Possibly none or all batch rows | Outcome not yet resolved | `COMMIT_ATTEMPTED` | `UNKNOWN` if queried before authoritative resolution | Stop; no retry |
| Commit returns nil; live process awaiting anchor acknowledgment | Batch data and `BatchCommitted` outbox event committed in one tx | Local receipt durability depends on configured sync policy; anchor ack not yet known | `COMMIT_CONFIRMED` | `N/A` while the run is live; do not publish complete | Do not begin another batch until anchor durably acknowledges and caller context remains active |
| Commit returns `ErrCommitUnknown` | Under A3, data and local receipt are both committed or both absent | Anchor has the prepare event only; an ambiguous Commit is never attested as committed | `COMMIT_UNKNOWN` | `UNKNOWN` | Stop; no data retry |
| Crash after Commit success, before per-batch anchor acknowledgment | Business data and local outbox receipt may persist atomically | Independent catalog shows no commit acknowledgment; local event is unanchored | `AUDIT_INCOMPLETE` | `UNKNOWN` | Never rerun Apply or reconstruct attestation from DB contents |
| Per-batch/terminal anchor request fails after DB event/seal is persisted | Current batch or seal may exist locally; no later business batch starts | Independent anchor has no matching acknowledgment | `AUDIT_INCOMPLETE` | `UNKNOWN` while local event is unanchored | Stop. Retry only the identical event while trusted process retains its observed Commit result; after process loss do not infer from mutable DB |
| Outbox write fails before Commit; rollback known (including ENOSPC) | Current batch has no business changes | Anchored prepare/prefix remains; no committed event | `FAILED_BEFORE_COMMIT` | `INCOMPLETE_VALID` | No fallback, no retry |
| Process crash after all batch acknowledgments but before terminal RunSeal/anchor seal | All confirmed batch data/receipts persist; no terminal seal | Independent catalog records only the acknowledged prefix | `AUDIT_INCOMPLETE` | `INCOMPLETE_VALID` | No post-restart completion reconstruction or business replay |

Linearization boundary for business data plus local receipt is the DB transaction commit, not external anchor acknowledgment. A DB-confirmed receipt means the database acknowledged that transaction and outbox event together; an independently anchored event is a separate later state. `Durable` requires verified engine/filesystem sync policy and is not assumed from Go `Commit()==nil` alone. Outbox error before commit causes rollback; anchor error after commit leaves that batch committed but the run incomplete and stops subsequent writes. `CommitUnknown` is only an error after underlying SQL Commit was attempted; validation/cancellation rejection before that is known pre-attempt failure.

## P1-3 consumer, backpressure and cancellation model

Candidate uses **C1 only**: a trusted internal DB outbox writer inside the transaction. No arbitrary caller callback, user-supplied channel, or third-party sink in first version. Queue capacity Q=0 (synchronous); no abandoned goroutine. Receipt insertion occurs before Commit in same transaction, so a slow DB insert delays/fails the still-uncommitted batch rather than creating a known committed/no-receipt window. Slow target/outbox operations backpressure by blocking the transaction and prevent the next batch from starting.

Context is propagated to DB operations. Cancellation before Commit attempts rollback; cancellation/driver error after commit attempt may be `CommitUnknown`. The implementation does **not** promise a hard finite wall-clock exit if a database driver, filesystem or kernel I/O fails to return. It does promise not to spawn and abandon a callback goroutine. If Owner requires a hard deadline, that requires separate driver/process isolation and must be designed; no goroutine timeout trick is acceptable. Panic containment is not relevant to user callback (none); internal code panic remains a defect, not a recover-and-continue condition.

## P1-4 persistent storage/file-system boundary

Per-batch receipts are not written to an arbitrary path, temp file, sidecar database, stdout or user-selected directory. They are rows in the already selected target DB, in the same transaction as changed business rows. Outbox is pre-provisioned by a separate reviewed migration, in an explicit reserved schema/table with unique event `(run_id,seq)` and `event_id`; batch-ID uniqueness is scoped to the prepare/terminal roles in §“Canonical envelope and event records”, not a blanket uniqueness on every event row. Append-only enforcement/grants are backend-specific, with retention/backup policy and no runtime DDL. This eliminates a separate receipt-path alias/no-clobber/symlink/rename/fsync protocol, but does not eliminate target DB file/path security.

For SQLite, require an owner-controlled private directory, existing regular target file, source-target distinct identity, no untrusted parent-directory writers, reject symlink/reparse paths and unexpected hardlink count at preflight, bind/recheck file identity before writer open and every transaction, and use local filesystems with tested SQLite lock/journal/fsync semantics. Because current `sqliteDSN` opens by pathname after `Stat` and does not pin an OS handle, TOCTOU hardening is **not implemented/proven**; do not claim resistance to a hostile concurrent path replacer until a platform-specific safe-open design is tested. Unsupported file systems/platform semantics fail closed. Windows requires native reparse-point/file-ID/ACL tests; Linux requires no-follow and inode/device checks; network filesystems are unsupported until tested.

For MySQL, no user filesystem audit path is added; require private database/schema, least-privilege grants, primary-only reads, transactional engine for every modified/outbox table, and verified durability configuration. Unsupported engine, replica, cross-database transaction or unverified commit durability is UNSUPPORTED. Existing MySQL adapter CI is not MySQL CLI/outbox evidence.

Disk/quota growth remains Θ(K) in the target database. Check budgets before any write where possible, but same-transaction DB errors still fail/rollback or produce CommitUnknown. Never delete/truncate prior receipts, fall back to memory or continue business writes after outbox failure. If target storage is full, stop; status is failed/unknown according to commit boundary. OS file cache is not counted as bounded application heap.

## API compatibility matrix (proposal only)

| Caller/data | Legacy `m1a-v1 Apply` | New streaming surface (candidate) |
|---|---|---|
| Existing `Plan` and signed approval | Continue to validate exactly as today; same Plan.ID and existing history record; complete `Committed` slice | Must not silently reinterpret. Either use the same Plan/approval only if Owner approves audit mode not needing a separate approval binding, or define a new streaming Plan/approval v2 bound to audit protocol/outbox identity. The latter preserves m1a path unchanged but requires explicit new signed record format. |
| Existing `Workflow` implementers | No new required method; preserve interface/return semantics | Add a separate versioned interface/type (not mutate old method). Do not change global `APIRevision` in place; additive parallel revision/surface requires Main-owned contracts proposal. |
| Receipts | All receipts in returned slice, O(K) | Bounded summary + queryable durable outbox/run handle; complete detail read through a paged iterator requiring bounded page size; no accumulation by default. |
| CLI | Legacy path remains explicit and documented O(K), or becomes deprecated only after Owner decision | CLI can select new protocol only via explicit flag/version, no automatic fallback. Apply authorization and signature checks remain mandatory. |
| Cross-use | m1a plan/approval valid only on legacy m1a path unless exact binding compatibility is proven | No v2 approval authorizes legacy path; no legacy approval authorizes altered audit protocol unless explicitly bound/approved. |

A global `APIRevision` bump changes the revision included in Plan identity and `Plan.ValidateShape` currently accepts only one constant; it can invalidate old Plan/Approval. Therefore “bump the version” is not a compatibility design. Exact names/DTO/signature coexistence require Owner approval and a separate contract proposal.

## Resource budget and proof boundary

Current proposal makes no numeric resource cap an approved requirement. Define `Lrow` (maximum bytes of projected values per row), `B`, `W`, `C`, `T`, `H`, `F` (filename bytes), `D` (schema metadata), `Q` (queue), `R` (runtime/driver/cache), and `K` (committed pages). Candidate outbox API has Q=0 and does not retain receipts in app heap. Required upper-bound shape:

`AppLive <= fixed + α·B·Lrow·Ccopies + β·W·Lvalue + metadata(T,C,D) + approvalIndex(H,F) + runtime/driver(R)`.

This is N-independent only if metadata, approval directory scan, row/value bytes, and runtime/driver terms are separately bounded or explicitly excluded from the claimed AC. Receipt storage in target DB remains disk Θ(K); OS cache is separately reported. No complete process RSS bound has been proven.

- Existing validated maxima: Apply `B≤1000`; worker count `W≤16`; reader requested pages ≤10000 but lookahead allocations are one item larger. These do not bound L/T/C/H/D.
- Approval history: retain duplicate-plan ambiguity protection but enumerate directory in fixed `ReadDir(n)` chunks, sum entry count and name bytes, fail before any target write if `H > Hmax` or `F > Fmax`. Proposed values only: `Hmax=10,000`, `Fmax=4 MiB`; test worst-case names and directories. The bound must apply before accumulating entries; simply call `ReadDir(0)` then check is not bounded.
- Proposed but unapproved strict-mode caps to investigate: `B≤64`, `W≤16`, `Lrow≤64 KiB`, `C≤32`, `T≤128`, `H≤10,000`, `F≤4 MiB`, schema metadata ≤16 MiB, Q=0. These are **PROPOSED — OWNER APPROVAL REQUIRED**, not product defaults or demonstrated safe budgets. Their product implies multi-copy page memory and must be measured; if too costly, lower caps or define less ambitious working-set claim.
- Single-value limits must be enforced before materializing large values where possible (DB-side byte-length predicate/adapter support). Enforcing after driver has already copied an unbounded blob is too late to prove a hard peak. Driver/native engine internal memory may still depend on value size and DB implementation; either bound it experimentally/contractually or exclude it from application-heap claim honestly.
- Plan scopes and schema enumeration must stop at caps before retaining unbounded maps/slices; otherwise T/C/D terms remain unbounded. Error/Issue aggregation must stay category-bounded or cap distinct column locations.
- Go runtime/GC and OS cache are not strict B×W variables. A data-working-set asymptotic claim can be proven from object lifetimes; an absolute process RSS guarantee cannot be claimed without a separately approved resource envelope and platform runtime evidence. GOMEMLIMIT is a soft runtime target, not a proof.

## Design-level failure-injection plan (not executed)

All rows below are **planned**, not passing tests. A prototype/model check was not run. Production Go tests, real SQLite/MySQL outbox transaction probes, crash/restart/power-loss tests, and platform filesystem tests remain UNVERIFIED.

| # | Initial state / injection | Expected DB state | Audit state/error | Continue? Retry? | Verification / contract |
|---:|---|---|---|---|---|
| 1 | PREPARED absent; fail before insert | no business change | no envelope/receipt; sanitized pre-write error | no | verify no Writer/UPDATE; m1a gates preserved |
| 2 | fail after PREPARED, before data tx | PREPARED only | run incomplete | stop; no data retry | reopen DB and inspect exact run/batch; new API only |
| 3 | fail before Commit / Update error | business tx rolled back | PREPARED, optionally ABORTED; no COMMITTED event | stop; no automatic retry | SQL readback and outbox transaction log |
| 4 | CommitUnknown | atomic all-or-none business+COMMITTED event | same-primary query: present=>committed; unresolved=>UNKNOWN | never blind retry | inject commit-before/after-durable acknowledgement |
| 5 | process crash after DB Commit success but before per-batch anchor acknowledgment | business+local outbox receipt may persist atomically; external catalog has only enrollment/prepare | `UNKNOWN`/`AUDIT_INCOMPLETE`; local event is not promoted from mutable DB after restart | no rerun or post-restart attestation reconstruction | child-process kill at exact seam; compare recovered DB and independent catalog without claiming completion |
| 6 | outbox INSERT partial/SQL error/ENOSPC | same tx rollback or CommitUnknown | no independent half-file; no success without committed row | stop | forced DB quota/full + target/outbox readback |
| 7 | file fsync / parent fsync failure | no separate receipt file; DB durability depends engine config | unsupported/unverified sync => reject before Apply; commit error=>unknown | stop | SQLite settings + fsync fault harness; MySQL durability probe |
| 8 | delete a middle outbox event whose commit is independently anchored | business rows unaffected after tampering | `TRUNCATED_OR_MISSING`; digest alteration instead is `TAMPERED_OR_UNTRUSTED` | no reapply | compare local sequence against independent catalog/anchor events |
| 9 | truncate tail / omit seal | database may contain committed prefix | anchored prefix with no expected terminal=>`INCOMPLETE_VALID`; catalog expects missing event/seal=>`TRUNCATED_OR_MISSING`; altered digest=>`TAMPERED_OR_UNTRUSTED` | no | verify classification precedence; anchored INCOMPLETE seal never returns COMPLETE_VALID |
| 10 | replace RunID or replay another run records | business state unchanged | run/plan binding or signature mismatch | no | canonical envelope and unique RunID tests |
| 11 | modify seal/signature | business state unchanged | invalid signature/anchor mismatch | no | independent public-key verification |
| 12 | rewrite a complete chain for a run that already has independently registered enrollment and acknowledged batch events | business state unchanged | rewritten local events mismatch the external catalog/anchored sequence => `TAMPERED_OR_UNTRUSTED` | no | attacker can write target DB but cannot compromise trusted Runner identity or anchor; separately document pre-enrollment rewrite and host/Runner compromise as excluded threats |
| 13 | internal sink blocks/slow | current batch uncommitted while INSERT blocks | ctx-aware driver returns; otherwise call may hang (no hard deadline guarantee) | do not abandon goroutine | slow DB and cancellation tests, monitor same caller goroutine |
| 14 | context cancellation | precommit rollback; after commit attempt unknown possible | canceled/unknown as appropriate | no automatic retry | assert no detached goroutine, writer closes when driver returns |
| 15 | repeated RunID, event ID, or duplicate prepare/terminal batch role | unique constraints reject before conflicting records; verifier also rejects illegal transition | conflict; no overwrite | no | test `(run_id,seq)`, `(run_id,event_id)`, and batch role uniqueness across SQLite/MySQL |
| 16 | symlink/hardlink/path replacement | unsupported path refused before writer | no audit path created | no | native Linux/Windows path identity and race tests; until then private trusted directory assumption |
| 17 | approval directory > Hmax/Fmax | no target transaction opened | sanitized resource-limit refusal; approval ambiguity check preserved | no | directory with Hmax+1 entries; chunked scan memory cap |
| 18 | value/row > Lrow | preflight/reader refuses before data update | resource-limit error; no receipt | no | oversized text/blob on SQLite/MySQL; ensure not copied unbounded before check |
| 19 | slow DB/outbox backpressure | no next batch begins; current tx state follows DB | cancellation/failure/unknown | no automatic retry | event ordering and connection stats |
| 20 | second batch failure after first committed | first business+receipt atomic; second rolled back/unknown | partial/unsealed run, first receipt remains | no rerun | readback both tables, verify exact chain and no false Complete |
| 21 | process crash after every batch anchor ack but before terminal RunSeal/anchor seal | anchored batch prefix and data remain | `INCOMPLETE_VALID` if catalog confirms only the prefix and no later terminal expectation; never `COMPLETE_VALID` | no post-restart completion reconstruction | kill after final batch ack, verify exact status and no second business write |

## Other Issue #13 evidence gaps

Still not accepted as complete: only one 1M trial per configuration; no 10k–1M CLI subprocess scale matrix; native Windows/macOS performance absent (Windows CI tests are not perf; cross-build is not native); full-flow B/op/allocs/op missing; PR70 preflight TMPDIR/cgroup P2; child env/output sanitization; unvalidated environment prose; 100ms sample misses short peaks; phase-5 sampler memory grows with duration; evidence path/source-hash coupling; no MySQL CLI E2E (adapter CI is not CLI). Reassess each against the original Issue text, do not expand to #14 or erase required AC.

## Implementation boundary and authorization

This ADR recommends only a candidate. **No production files are authorized.** Prohibited without explicit further authorization: `pkg/contracts/**`, `APIRevision`, runner Apply/receipt semantics, DB writer/commit behavior, Worker Pool, CLI write gates, History production format, BatchSize defaults, release/tag. No #14 checkpoint/resume/exactly-once work.

If Owner approves ADR-01..ADR-11 (including threat scope) and separately authorizes implementation, estimate at least four sequenced packages: (1) contract/approval coexistence proposal; (2) DB outbox schema and transaction owner implementation; (3) runner/CLI streaming API integration; (4) external audit anchor/seal/verifier + platform security, with independent QA/Security. Split/serial ordering must follow actual file ownership. This is an estimate, not authorization.

## Owner decision signature block

No approval is recorded here. The Issue Owner must explicitly select or reject each ADR-01..ADR-11; empty signature means pending.

- Issue Owner decision reference/comment: **PENDING OWNER DECISION**
- Audit threat model / pre-enrollment and host-root boundary (ADR-11): **PENDING OWNER DECISION**
- AC scope (S1/S2/S3): **PENDING OWNER DECISION**
- Audit guarantee (A1/A2/A3): **PENDING OWNER DECISION**
- Outbox/schema change: **PENDING OWNER DECISION**
- External trust anchor: **PENDING OWNER DECISION**
- Consumer model: **PENDING OWNER DECISION**
- Platform/storage support: **PENDING OWNER DECISION**
- Numeric memory/disk/history limits: **PENDING OWNER DECISION**
- API/Plan/Approval compatibility: **PENDING OWNER DECISION**
- Production file authorization: **PENDING OWNER DECISION**
