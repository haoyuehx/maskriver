# Issue #14 WP0 — SQLite Local Transactional Recovery Protocol v1

**Status:** `WP0 ACCEPTED — INITIAL AND SECOND-CONTEXT REVIEWS RECORDED; P1 CLOSED`
**Issue:** [#14](https://github.com/haoyuehx/maskriver/issues/14)
**Baseline `main`:** `aee0b801c738b89a6267e06c575c95878f785c7f`
**Protocol revision:** `m2-recovery-v1`
**Recovery schema version:** `maskriver-recovery-v1`
**Streaming protocol:** `m2-stream-v1` (unchanged)
**Approval binding:** `m2-approval-binding-v1` (unchanged)
**Shared API revision:** `m1a-v1` (unchanged; `pkg/contracts` is not modified by WP0/WP1)

This document freezes the recovery design before implementation. It states what is
atomic, what is idempotent, what is deliberately *not* claimed, and which failure
modes must fail closed. WP1–WP4 implement and test this document; they may not
weaken it.

---

## 1. Scope and supported boundary

Supported recovery target for this round:

- **Linux amd64, local SQLite only**, `journal_mode=delete`, session
  `synchronous=FULL`, `foreign_keys=ON`, `query_only=OFF`. These are the
  already-verified capabilities of the `maskriver-outbox-v1` streaming path
  (Issue #13). Other platforms, WAL, MySQL and remote databases are
  `ErrUnsupported` and must fail closed.
- The plan uses the **existing pure deterministic strategies** in
  `internal/mask`: the replacement is a pure function of
  `(typed value, strategy ID+version, scope, normalization version, key ID, key bytes)`.
  The runner has no mapping-store seam on the streaming path; `pageChanges`
  calls `maskBatch` and never consults `MappingReader`/`MappingWriter`.
- The source is an immutable, quiescent rollback-journal snapshot whose
  SHA-256 is bound into the plan (`SourceSnapshotID`). The target is a separate
  local copy bound by `TargetID` (path digest) plus a recorded file identity
  (`device:inode`).
- Every commit is a short SQLite transaction. Recovery is **local and
  single-node**; there is no distributed or multi-process exactly-once claim.

Explicit non-goals: MySQL recovery, Windows streaming recovery, WAL recovery,
cross-store atomicity, anonymous/global unique replacement, and any
"exactly-once delivery" claim outside the supported local SQLite transaction.

---

## 2. Threat model

Assets: the source values, the masking key, the integrity of the target rows,
and the integrity of the committed-range record.

Trusted within the supported boundary (same as Issue #13 ADR-11):

- the local directory and the process account that can write the target file;
- the OS file system's atomic rename and SQLite's rollback-journal atomicity;
- the deployment-provisioned Ed25519 trust file and the reviewer key's
  provisioning separation.

Adversaries and hazards considered:

| Threat | Handling |
|---|---|
| Process crash / `SIGKILL` at any phase | SQLite atomic commit or rollback; recovery re-reads the authoritative DB, never a local progress file |
| Ambiguous commit error | `COMMIT_UNKNOWN`; no retry; next resume reconciles from the DB, not from error text |
| Double masking by replaying a committed batch | Committed prefix is verified against deterministically recomputed replacements and skipped; conflicting state is `CONFLICT` |
| Forged/stale run identity | Manifest binds plan content, source snapshot, target identity, strategy/key/normalization, approval binding, budget and a key-check value |
| Re-issued approval | The manifest binds the approval *binding revision* and canonical plan content, not one record identity. A newly signed, currently valid approval for exactly the same plan content may authorize resume; it is re-verified (signature, trust, expiry, revocation) on every resume. A revoked record is refused unless the store holds a different valid record |
| Expired/revoked approval | Full streaming Ed25519 verification runs before any DB write on every resume |
| Corruption or tampering of progress/receipt tables | Continuity, CAS sequence, cross-check of progress against receipts, and exact schema-shape verification; fail closed |
| External modification of the target | Prefix/suffix value verification; any divergence is `CONFLICT`; never overwritten |
| Sensitive data written to the manifest | Manifest contains identifiers, digests, counts and timestamps only |
| Path replacement of the target | Existing preflight + identity recheck; not proven TOCTOU-proof (documented limitation) |
| Concurrent second resume | Optimistic CAS on `checkpoint_sequence` inside the same transaction; loser gets `CONFLICT` and rolls back |

Not defended against: a privileged attacker who can rewrite the target and the
recovery tables consistently, or who controls the trust file *and* the reviewer
key. These are outside the local signature boundary and are stated, not solved.

---

## 3. Storage model

All recovery state lives **inside the target SQLite database**, so the business
UPDATE, the batch receipt and the checkpoint update can share one transaction.
Two explicit, pre-provisioned schemas are used:

1. `maskriver-outbox-v1` — unchanged Issue #13 tables
   (`maskriver_outbox_schema`, `maskriver_outbox_run`, `maskriver_outbox_batch`).
2. `maskriver-recovery-v1` — new additive tables:
   - `maskriver_recovery_schema(version, created_at)`
   - `maskriver_recovery_manifest(...)` — one row per run
   - `maskriver_recovery_progress(...)` — append-only committed progress log

Run metadata is created in one frozen order: `InitializeRun` (outbox v1 run row)
first, then `CreateManifest`. The manifest has a foreign key to the outbox run,
so a manifest without its run row is impossible; the reverse crash window
(outbox row without manifest) is safe and reported as **not recoverable**
(`status` exit 7 / `invalid manifest`), never as `new` or resumable. No repair is
attempted automatically.

Migration is explicit and separate from Apply: `InstallOutboxSchema` and the new
`InstallRecoverySchema` run only from explicit management commands
(`init-outbox`, `init-recovery`). No Apply/Preview/Status path executes DDL.
Installing the recovery schema never alters or deletes outbox v1 data. Total
absence of every recovery table means the ordinary legacy streaming path (no
silent "recoverable" claim). Any recovery table or version marker present but
not exactly matching the v1 shape is `ErrUnsupported` for Apply, resume and
status; there is no partial/legacy fallback. A run that has an outbox row but no
recovery manifest is **not** recoverable and is never presented as such.

Reserved-table policy: recovery table names join the reserved list; business
scopes and plans naming them are refused with `ErrUnsafe` (`db.IsOutboxTable` is
extended to cover both schema families; `db.OutboxTables()` keeps its v1 meaning
for the outbox migration).

### 3.1 Manifest (non-sensitive)

| Column | Meaning |
|---|---|
| `run_id` | 32-hex run identity, primary key |
| `protocol` | `m2-recovery-v1` |
| `streaming_protocol` | `m2-stream-v1` |
| `approval_binding` | `m2-approval-binding-v1` |
| `plan_id` | frozen `m1a-v1` plan ID |
| `plan_content_digest` | signed canonical plan content digest (history) |
| `scope_digest` | ordered scope-set digest |
| `source_snapshot_digest` | SHA-256 of the immutable source file = `Plan.SourceSnapshotID` |
| `target_id` | `Plan.TargetID` (target path digest) |
| `target_identity` | `device:inode` of the target file at run creation |
| `schema_digest`, `config_digest` | frozen plan digests |
| `strategy_id`, `strategy_version` | single strategy tuple |
| `normalization_version` | e.g. `typed-lexical-v1` |
| `key_id` | key identifier label, never key material |
| `key_check` | KCV = HMAC-SHA256(key bytes, `"maskriver-recovery-v1/key-check\0" || key_id`); binds the actual key without storing it. The key file must be a CSPRNG-generated >=32-byte secret, not a passphrase; the KCV is an offline guessing oracle for low-entropy keys |
| `settings_digest` | canonical digest of `ResourceBudget` + `BatchSize` |
| `state` | recovery state machine value |
| `business_complete` | 0/1, set only by the final boundary transaction |
| `validation_state` | `none` / `passed` / `failed` |
| `checkpoint_sequence` | CAS token and latest committed progress sequence |
| `created_at`, `updated_at` | UTC timestamps |

Forbidden in the manifest: original values, replacements, plaintext primary
keys, keys/seeds, DSNs, or any material from which a sensitive value can be
recovered.

### 3.2 Progress log (the recovery marker)

Append-only, one row per **committed** progress event:

| Column | Meaning |
|---|---|
| `run_id`, `checkpoint_sequence` | primary key; sequence 1..N, strictly +1 |
| `kind` | `batch` (business rows changed), `progress` (no-op page), `boundary` (table finished / business complete) |
| `table_ordinal` | 0-based index into `Plan.Tables`; `len(tables)` after the last table |
| `rows_processed` | cumulative rows consumed **since this table's boundary event** (includes all-NULL/no-op rows); reset to 0 by the boundary event that enters the next table |
| `rows_committed` | cumulative rows actually updated across the whole run |
| `committed_batches` | cumulative business batches (`kind=batch`) |
| `batch_id` | run-scoped batch ID for `kind=batch`, empty otherwise |
| `affected_rows` | rows changed by this event (0 unless `batch`) |
| `created_at` | UTC timestamp |

`rows_processed` and `rows_committed` are deliberately distinct: a page whose
planned columns are all NULL advances progress without pretending to have
written rows. No primary-key values are stored. The cursor is reconstructed by
re-scanning the immutable source with bounded keyset pages.

`checkpoint_sequence` counts every committed progress event (`batch`, `progress`
and `boundary`). The outbox `batch_sequence` stays **dense and independent**:
only `batch` events insert a receipt, starting at 0, so `ReceiptReader` keeps its
existing contiguous-page contract. `progress`/`boundary` events never insert a
receipt. A re-executed sequence regenerates the same run-scoped `batch_id`; a
committed sequence is never executed again.

### 3.3 Normative v1 schema constraints

`internal/db/recovery_schema_v1.sql` is the normative DDL (WP1 embeds it). The
v1 shape is frozen as:

- all three tables are `WITHOUT ROWID`; the schema marker has a single row;
- `maskriver_recovery_manifest.run_id` is the 32-hex primary key with a
  `FOREIGN KEY` to `maskriver_outbox_run(run_id) ON DELETE RESTRICT`; `protocol`,
  `streaming_protocol`, `state` and `validation_state` are `CHECK`-constrained
  to the values in this document; `key_check` is 64 lowercase hex characters;
  `business_complete` is `CHECK (0,1)`; `checkpoint_sequence >= 0`;
- `maskriver_recovery_progress` has `PRIMARY KEY (run_id, checkpoint_sequence)`,
  a foreign key to the manifest, `checkpoint_sequence >= 1`, `kind IN
  ('batch','progress','boundary')`, non-negative counters, and `CHECK`s that a
  `batch` has `affected_rows > 0` and a non-empty `batch_id` while
  `progress`/`boundary` have no batch id and zero affected rows, and that
  `business_complete = 1` only on a `boundary` event;
- string lengths are bounded; timestamps are UTC RFC3339Nano.

Any column/order/type/NOT NULL/primary-key difference is a shape mismatch and is
refused, exactly like the outbox v1 shape check.

---

## 4. Transaction boundaries

Normal business page with at least one non-NULL planned value:

```text
BEGIN
  UPDATE target rows          (existing optimistic writeTx)
  INSERT outbox receipt       (existing maskriver_outbox_batch)
  INSERT recovery progress    (kind=batch)
  UPDATE recovery manifest SET checkpoint_sequence=?, state='running'
       WHERE run_id=? AND checkpoint_sequence=?     -- CAS
COMMIT
```

Page with nothing to write (zero pending `RowChange`s after the per-row
`assignsNothing` check, i.e. every planned value of every row in the page is
NULL). A mixed page is a `batch` event whose `affected_rows` is smaller than its
`rows_processed` increment; it never pretends every consumed row was changed:

```text
BEGIN
  INSERT recovery progress    (kind=progress, affected_rows=0)
  UPDATE recovery manifest ... CAS ...
COMMIT
```

Table boundary / business completion:

```text
BEGIN
  INSERT recovery progress    (kind=boundary, table_ordinal=next, rows_processed=0)
  UPDATE recovery manifest ... CAS ... [business_complete=1 on final boundary]
COMMIT
```

Validation and terminal state are read-only + one final metadata UPDATE after a
successful read-only strict `Validate`; they never mix with business writes.
A validation pass is recorded only after it actually passed.

The `m2-stream-v1` DTO/wire protocol is unchanged. With the recovery schema
installed, the transaction additionally writes the progress row and manifest CAS,
so recovery-table integrity becomes a precondition of streaming Apply (fail
closed, never silently skipped). Without the schema the transaction is exactly
the Issue #13 transaction.

Invariants proven by construction and by WP1 tests:

- business UPDATE, receipt and progress are in **one** SQLite transaction and
  **one** database;
- a pre-commit error rolls back all three;
- a successful commit makes all three visible together;
- there is no normal success path in which business data committed but the
  checkpoint is behind, or the checkpoint advanced without the business write;
- an error from `Commit` is always `ErrCommitUnknown` and never guessed;
- recovery writes keep the existing optimistic `Expected = source` compare and
  exactly-one-row enforcement; a tap that changes a row between verification and
  commit is rolled back, never overwritten by a PK-only UPDATE;
- the manifest CAS makes a concurrent replay of the same sequence fail closed;
- the CAS `UPDATE ... WHERE checkpoint_sequence = expected` must affect exactly
  one row; zero rows is `CONFLICT` and rolls the whole batch back, so a progress
  row can never commit while the manifest token stays behind;
- resume requires `manifest.checkpoint_sequence == max(progress.checkpoint_sequence)`
  **and** a contiguous `1..N` progress log; a manifest restored behind the log,
  or a log with a gap, is `CONFLICT`;
- the manifest's duplicated run identity (`plan_id`, `target_id`,
  `plan_content_digest` vs the outbox `scope_digest`, `approval_binding`,
  streaming protocol) is cross-checked against `maskriver_outbox_run`; divergence
  is `CONFLICT`;
- state updates also use a checkpoint-sequence CAS. A stale finaliser changes
  zero rows and gets `CONFLICT`; `conflict`, `aborted`, `complete` and
  `validation_failed` are terminal and immutable within one `run_id`.

---

## 5. Idempotency argument

For a supported run, the expected replacement of a source value depends only on
the immutable source snapshot, the frozen plan (strategy, scope, normalization,
key ID), the approval-bound plan content and the key bytes. There is no
cross-process mapping state. Therefore recovering a committed region can be
done by **recomputation and comparison**:

- For rows before `rows_processed` in the current table (and every row of every
  earlier table), the target must equal the deterministic replacement of the
  source value (NULL stays NULL). The supported apply path already refuses a
  non-NULL replacement equal to the source (`runner.maskColumn`), so a committed
  non-NULL row necessarily differs; the recovery check is equality with the
  recomputed replacement, not a separate inequality rule. This keeps a
  legitimate fixed-point replacement from being misclassified as a conflict.
- For rows at or after `rows_processed`, the target must still equal the source
  value.
- If a row beyond the checkpoint is already masked, or a row inside the
  committed prefix does not match the recomputed replacement, the state is
  `CONFLICT`; no retry and no overwrite. This verification covers planned
  columns plus full row-count and bidirectional key correspondence; edits to
  non-planned columns are outside the masking contract and are not claimed to
  be detected.
- A claimed `rows_processed` greater than the source row count of that table, or
  a final boundary that does not equal the source row count, is `CONFLICT`.

Tables at or before the checkpoint are verified in one bounded pass. Later
unvisited tables are verified page-by-page immediately before their first write;
a conflict there is detected before that page is written, but earlier tables may
already have been continued, so the guarantee is "never overwrite an unverified
page", not "verify every later table before any write".

Committed batches are therefore skipped, not re-written. A replayed batch would
have to pass the same prefix verification, which is exactly what proves it must
not be replayed. Because replacement is a pure function, the final target of an
interrupted+resumed run equals the final target of an uninterrupted run; WP4
proves this by comparing A/B results.

This is **effect idempotency inside the supported local transaction**, not a
general exactly-once delivery claim.

---

## 6. Recovery state machine

States stored in the manifest:

| State | Meaning | Resume allowed |
|---|---|---|
| `new` | manifest created, no progress committed | yes |
| `running` | a process started/continued the run | yes after reconciliation (CAS prevents concurrent writers) |
| `recoverable` | known interruption, authoritative state consistent | yes |
| `recovery_required` | business work needs re-verification before continuing | yes (verification still mandatory) |
| `commit_unknown` | a commit attempt's outcome was ambiguous | yes, by reconciling the database; never by retry-on-error |
| `conflict` | target/checkpoint/receipt disagree or external modification | **no**; requires operator investigation |
| `validation_failed` | strict Validate failed after business completion | **no**; terminal for resume within this run, requires operator investigation / a new run |
| `complete` | business + audit + strict validation complete | yes, idempotent read-only re-confirmation |
| `aborted` | explicitly abandoned | no |

State is informational; **the authoritative decision is always re-derived from
the committed progress log and the target contents**. A stale `running` state
after a crash never authorizes a write by itself.

`business_complete`, `validation_state` and `state=complete` are distinct:
a run may have all business rows committed but not yet validated, or validated
but not yet marked complete. Only the full sequence produces `complete`.

---

## 7. Commit gap policy

`internal/mask.MemoryMapping` is an in-process test helper, not a verified
durable mapping store, and the streaming runner has no mapping-store seam. The
supported recovery path is limited to replacement that is **deterministically
recomputable** from source + frozen plan + key. That makes the mapping/target
commit gap empty for the supported path: there is no independent store whose
commit can diverge from the target transaction.

Policy:

- `m2-recovery-v1` carries an explicit allowlist of recomputable strategy IDs
  (`null`, `blank`, `redact`, `format_random`, `fake_email`, and the nine
  `cn_*` strategies). A plan naming any other strategy is `ErrUnsupported` when
  recovery is enabled; adding a strategy requires adding it to the allowlist and
  proving purity. If a future strategy requires cross-process mapping state,
  resume must **fail closed** unless a separate approved design binds that state
  to the target commit (for example, the mapping rows living in the same SQLite
  database and transaction).
- Writing an extra side file and calling it atomic is forbidden.
- `m2-recovery-v1` makes no generic distributed exactly-once or cross-store
  atomicity claim. Any such claim requires a new protocol revision and evidence.

---

## 8. Primary-key privacy policy

No plaintext primary-key value is persisted in the manifest or progress log.
Progress is expressed as table ordinal, row counts and sequence numbers.
Recovery reconstructs the keyset cursor by scanning the immutable source with
bounded pages and consuming exactly `rows_processed` rows. Hashing raw primary
keys is not treated as automatically safe, so it is not used. Digest fields in
the manifest are over plan/schema/scope/budget metadata, not over row values.

---

## 9. Source/target consistency

On resume, in fixed order (matches `StreamingResume`):

1. structural protocol/run/budget gates and the protocol-bound approval
   structure (including expiry/revocation via the history store);
2. plan rebuilt from the immutable source and the whole-plan static preflight
   (schema, primary key, scope order, constraints, triggers, incoming refs);
3. settings digest, plan-content/scope digests and the recomputable-strategy
   allowlist;
4. open the outbox and require the exact recovery schema (no DDL);
5. read the manifest and cross-check the duplicated run identity against
   `maskriver_outbox_run` (plan, target, plan-content/scope digest, approval
   binding, streaming protocol);
6. re-derive the source snapshot digest and compare it with the manifest/plan;
   the digest is an identity check, not a TOCTOU-proof reader binding, so the
   source must remain a quiescent isolated copy (the existing plan-builder caveat
   applies unchanged);
7. verify strategy / normalization / key ID / key-check value (KCV), target
   identity (`device:inode`), approval binding and budget/settings binding; the
   KCV is an offline oracle by construction, acceptable because the key is a
   high-entropy >=32-byte secret and it is the only way to bind key material
   without storing it;
8. verify progress-log continuity and cross-check every batch event against the
   same-ordinal receipt (id, affected rows, run-scoped batch id), both bounded;
9. verify the committed prefix/suffix of every table at or before the
   checkpoint; unvisited later tables are verified page-by-page immediately
   before each write, never after;
10. only then `state=running` and a new business transaction.

Static preflight still runs: schema, primary key, scope order, plan identity,
constraints, triggers, incoming references. The ordinary full dry run (which
assumes an unmasked target) is replaced by the prefix/suffix recovery
verification; it is not disabled or bypassed. Verification reads through the
same byte-aware bounded reader (`PageWithin`, `MaxRowBytes`) with fixed 128-row
pages, so its memory does not grow with the run; apply-page process/disk
admission still runs for every written page.

---

## 10. Failure recovery matrix

| Failure | Expected behaviour |
|---|---|
| Crash before first batch | no progress; nothing to skip |
| Crash after UPDATE, before commit | transaction rolled back; progress unchanged; batch is re-executed |
| Crash after receipt INSERT / checkpoint, before commit | same as above; all three rolled back |
| Real commit succeeded, response lost | progress + receipt + rows are present; resume verifies and skips |
| `Commit` returns error (ambiguous) | `commit_unknown`; stop; next resume reconciles from DB |
| Progress sequence duplicate/gap | `conflict`; refuse |
| Receipt missing for a committed batch | `conflict`/`recovery_required`; refuse |
| All-NULL page | progress-only transaction advances `rows_processed`, no fake committed rows |
| Crash between tables | next table reconstructed from boundary, no re-write |
| Crash after all business commits, before validate | business_complete true, validate re-run read-only |
| Crash after validate pass, before state=complete | resume re-validates and finalizes |
| Validate fails | `validation_failed`; never `complete` |
| Source changed | refuse |
| Target externally changed | `conflict`; refuse, never overwrite |
| Approval expired/revoked | refuse before any DB write |
| Budget/settings differ | refuse |
| Concurrent resume | CAS conflict; one winner |
| `SQLITE_FULL` | rollback; committed evidence retained; fail closed |
| Context cancel | stop safely; committed prefix remains recoverable |
| Unsupported platform/journal/missing schema | `unsupported`; no fallback |

---

## 11. Schema upgrade and backward compatibility

- `maskriver-outbox-v1` is not modified. Recovery tables are additive and
  versioned separately.
- `InstallRecoverySchema` requires the exact outbox v1 shape, then creates the
  recovery tables in one transaction. Equal version + exact shape is a no-op;
  any other shape/version is `ErrConflict`/`ErrUnsupported`; no auto-repair.
- `OpenRecovery` re-verifies both schemas, so a retained version marker cannot
  legitimise altered tables.
- Existing Issue #13 runs have no manifest and are reported by `status` as
  **not recoverable**. They are never silently upgraded or retried.
- Non-recovery `mask --streaming --apply` behaviour is unchanged when the
  recovery schema is absent; with the schema present it additionally records a
  manifest/progress, which is the only way a new run becomes resumable.
- `pkg/contracts` is untouched; recovery types are internal (`internal/db`)
  plus runner/CLI wiring. They are not a public SDK.

---

## 12. Threat/evidence deliverables (WP1–WP4)

- WP1: versioned schema, migration tests (install, reinstall, wrong version,
  rollback), same-transaction UPDATE+receipt+checkpoint, no-op progress,
  CAS/duplicate sequence, commit-unknown, reopen, legacy outbox compatibility,
  reserved names, bounded reads.
- WP2: recovery runner with the fixed gate order, prefix/suffix verification,
  commit-unknown reconciliation, multi-table/NULL continuation, bounded memory,
  repeated resume idempotency.
- WP3: read-only `status`, explicit `--resume` requiring `--apply` and a valid
  approval, distinct exit codes, no auto run/rename/DDL.
- WP4: deterministic fault injection at before/after update, receipt,
  checkpoint, commit and validate; process-level CLI interruption at batches
  1/10/100/500, between tables, during the second table, after business
  completion, after validation; A/B result equality; security tests.

---

## 13. Concrete interface list (implemented in WP1–WP3)

`internal/db` (new, internal):

```go
const RecoveryProtocolRevision = "m2-recovery-v1"
const RecoverySchemaVersion    = "maskriver-recovery-v1"

type RecoveryState string // new/running/recoverable/...
type ValidationState string // none/passed/failed

type RecoveryManifest struct { /* non-sensitive fields of §3.1 */ }
type RecoveryProgress struct { /* fields of §3.2 */ }

func InstallRecoverySchema(ctx context.Context, opts OutboxOptions) error
func (o *Outbox) RecoveryAvailable(ctx context.Context) (bool, error)
func (o *Outbox) OpenRecovery(ctx context.Context) (*Recovery, error)

func (r *Recovery) CreateManifest(ctx context.Context, m RecoveryManifest) error
func (r *Recovery) Manifest(ctx context.Context, runID string) (RecoveryManifest, error)
func (r *Recovery) ProgressPage(ctx context.Context, runID string, after int64, limit int) ([]RecoveryProgress, bool, error)
func (r *Recovery) BeginRecoveryBatch(ctx context.Context, runID string, plan contracts.TablePlan,
    receipt contracts.StreamingBatchReceipt, progress RecoveryProgress) (contracts.WriteTx, error)
func (r *Recovery) CommitProgress(ctx context.Context, runID string, progress RecoveryProgress) error
func (r *Recovery) SetManifestState(ctx context.Context, runID string, state RecoveryState,
    businessComplete bool, validation ValidationState) error
```

`internal/runner` (new internal entry points):

```go
type StreamingRecoveryRequest struct {
    Protocol string; Plan contracts.Plan; Approval contracts.StreamingApproval
    ExplicitApply bool; RunID string; BatchSize, ReceiptPageSize int
    Budget contracts.ResourceBudget
}

func (e *Engine) StreamingResume(ctx context.Context, req StreamingRecoveryRequest)
    (contracts.StreamingApplySummary, contracts.RecoveryReport, error)

type RecoveryReport struct { /* table ordinal, rows_processed, rows_committed,
    committed_batches, checkpoint_sequence, business_complete, validation,
    state, resume_eligible; no values/keys */ }
```

`cmd/maskriver` (new CLI surface):

```text
maskriver init-recovery --target <existing-db>
maskriver status --target <existing-db> --run-id <32-hex>
maskriver mask --streaming --apply --resume --run-id <32-hex> ... (existing gates)
```

Exit codes: `0` complete; `1` generic refusal/incomplete; `2` usage;
`3` recoverable/interrupted; `4` conflict; `5` commit unknown;
`6` unsupported; `7` invalid manifest; `8` unauthorized; `9` validation failed.
`status` opens the target with SQLite `mode=ro`, verifies the exact schemas,
issues no DDL and no session-PRAGMA write, and never triggers recovery. A test
asserts the target bytes (and absence of new journal files) are unchanged.

---

### 13.1 Retention of recovery metadata

After completion the masked database retains the `maskriver_recovery_*` rows
(run ID, plan/scope digests, counts, timestamps, KCV) and the outbox receipts.
They are never auto-dropped. Downstream holders of a masked artifact therefore
also hold this non-sensitive recovery metadata and a key-check oracle; a later
run on that file must treat the names as reserved. Dropping them is an explicit
operator action outside this protocol. Keys must be high-entropy (CSPRNG >=32
bytes), otherwise the retained KCV weakens offline guessing resistance.

## 14. Explicit non-claims

- No general exactly-once, no cross-store/distributed atomicity, no MySQL or
  Windows recovery, no WAL recovery, no power-loss durability beyond SQLite's
  verified `synchronous=FULL` local behaviour and its documented limits.
- No guarantee that an attacker with write access to both the target and the
  recovery tables cannot fabricate a consistent-looking state.
- No claim that a simple `last_key` file is safe; this protocol never uses one.
- No automatic repair of conflicts, no `--force` bypass, and no claim of
  completion without a real strict Validate pass.
- Approval validity (expiry/revocation) is checked once at resume start, exactly
  like Issue #13's one-shot apply; a very long resume is not continuously
  re-authorized. This is stated, not hidden.
- The key-check value is an offline verification oracle for a high-entropy key;
  it binds key material but is not a key-storage or key-rotation mechanism.
