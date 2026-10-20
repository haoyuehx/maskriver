# Issue #13 Owner direction — 2026-10-20

**Status:** `DECIDED — DESIGN DIRECTION RECORDED; IMPLEMENTATION GATES UNCHANGED`.

**Decision source:** the repository owner, in the task instruction that delegated architecture
decisions to the AI technical lead / Orchestrator for Issue #13. This file records that
delegated direction. It is **not** a GitHub review signature, **not** a maintainer
attestation, and **not** evidence that any code has been implemented, tested, or verified.

**Scope of what changed:** design documentation only, plus the recorded acceptance-scope
revision in Issue #13. No production code, no `pkg/contracts` change, no transaction
behaviour, no CLI write authorization, no default, no release or tag changed in this round.

## Provenance and separation of records

| Layer | Status for this round |
|---|---|
| Technical decision (delegated by repository owner to AI technical lead) | **RECORDED** in this file |
| `AGENTS.md` maintainer authorization for restricted file sets | **WP1 only, limited**: new versioned contract types and tests under `pkg/contracts/**`; all other modules remain unauthorized |
| Design review (independent, read-only) | Performed on the exact PR heads; see `docs/reviews/` |
| Production implementation verification | **NOT STARTED** — WP2/WP2b/WP3/WP4 are not authorized |
| GitHub Owner approval signature | **NOT PROVIDED** — must not be cited as one |

Where repository process requires a maintainer-specific record, this file states the
limitation instead of inventing one. The Issue #13 scope revision is posted as a separate,
clearly labelled record; the original acceptance text is preserved verbatim.

## ADR-01 — S2 approved direction

**APPROVED DIRECTION:** a new, explicitly versioned streaming Apply path carries the strict
memory bound. The legacy `m1a-v1` Apply API keeps its full-receipt semantics.

- Legacy `Apply` returns `ApplyReport.Committed []BatchReceipt` and therefore O(K) receipt
  memory. K is the number of successful non-empty committed pages.
- The new M2 streaming path must keep the execution/return working set bounded and must not
  retain all receipts in memory.
- The new path must prove itself on real SQLite, through the CLI, under cancellation, and at
  high load before any acceptance claim.
- Documentation must always distinguish Legacy Apply from Streaming Apply.
- Issue #13 stays OPEN until the new path's acceptance evidence exists.

This is an explicit acceptance-scope revision, not a claim that the original global AC is met.

## ADR-02 — legacy API compatibility

**APPROVED DIRECTION:** keep the legacy API fully compatible.

Unchanged: `ApplyReport` shape and `Committed` semantics, Plan.ID computation, approval
binding, existing `BatchReceipt` field meaning, legacy M1 CLI default behaviour, and
`pkg/contracts.APIRevision == "m1a-v1"`. Documentation must label the legacy path O(K).
No silent switch of legacy callers onto the streaming protocol.

## ADR-03 — A3 approved direction for the new path

**APPROVED DIRECTION:** the new streaming path uses a same-target-database transactional
outbox: business UPDATEs and the corresponding committed-batch receipt INSERT commit or roll
back in one target-DB transaction, with these consequences:

- A known Commit failure never produces a confirmed receipt.
- `CommitUnknown` stays UNKNOWN; no automatic business retry.
- Partial completion is reported accurately.
- Sink/persistence failure can never be reported as complete success.
- No atomicity is assumed across databases, files, or services.

This is local transactional consistency only. Power-loss durability, external tamper
evidence, crash recovery and exactly-once are **not** implied and require separate proof.
WP2 implementation is not authorized in this round.

## ADR-04 — pre-provisioned outbox schema

**APPROVED DIRECTION:** the streaming path may use explicit, pre-provisioned audit tables
under all of: separate migration/deployment, no runtime DDL during Apply, versioned and
name-collision-resistant reserved table names, no silent mutation of legacy M1 databases,
operator-visible target schema change, and fail-closed behaviour when the table is absent or
the version does not match. Outbox presence must not be achieved by skipping business schema
validation. Migration, privileges, retention and deletion policy need separate review.
This round only allows describing the capability in contracts; no database table is created.

## ADR-05 — independent anchor deferred

**DECISION: NOT REQUIRED FOR ISSUE #13.** No audit server, external trust anchor,
independent catalog, long-term archive, or active tamper-detection infrastructure is built in
this scope. Consequently no claim may be made about resistance to malicious tampering,
detection of whole-run deletion, protection from host/root compromise, or externally
verifiable audit proof. If those capabilities are later required, they need a new issue and
threat model. WP4 is not required work for #13.

## ADR-06 — no arbitrary third-party consumer

**APPROVED DIRECTION:** the first version uses only a controlled internal streaming
mechanism. No arbitrary external Go callback may enter the commit critical path: a callback
can block indefinitely, ignore context, re-enter, or panic, so no exit-time guarantee is
possible. A controlled internal sink, paged receipt reader, or database outbox reader is
allowed if it has explicit lifecycle, bounded queue or synchronous delivery, context
cancellation, resource cleanup, backpressure, failure propagation, and no silent receipt
loss.

## ADR-07 — platform scope

**APPROVED DIRECTION:** first production target is Linux amd64, local SQLite, independent
source/target databases, verified journal/synchronous configuration, and real same-database
same-transaction outbox. `journal_mode=DELETE` with `synchronous=FULL` is the first candidate
to verify experimentally; durability must not be declared proven before that experiment.

- Windows: legacy functionality and existing CI stay. Streaming outbox may get native
  compatibility tests, but the feature is **UNSUPPORTED** on Windows until native path
  identity, locking, filesystem and durability behaviour are verified. Wine or cross-build
  is not a substitute.
- MySQL: existing adapter behaviour is unchanged; MySQL streaming outbox is deferred to a
  separate WP2b. No MySQL CLI E2E or cross-backend transaction guarantee is claimed.

## ADR-08 — numeric budgets pending measurement

**PENDING MEASUREMENT AND OWNER APPROVAL.** Hard limits are required for BatchSize, worker
count, per-row and per-field bytes, table/column counts, approval-history entry and filename
bytes, sink queue capacity, process working set, and database/outbox disk budget. WP1 may
define the budget fields and their pure validation only. No numeric value here is approved,
measured, or a product default. WP3 production enablement requires real measurement and
numeric approval first.

## ADR-09 — parallel versioned contract

**APPROVED DIRECTION:** do not bump the global `APIRevision` in place. Keep `m1a-v1`; the
new path uses an independent protocol revision with explicit capability declaration, name to
be fixed by WP1.

- Legacy Plan.ID is unchanged.
- Existing signed approvals continue to validate on the legacy path.
- New Plan/approval bindings are explicitly bound to the streaming protocol.
- Approvals cannot be reused across mismatched protocols.
- Legacy authorizations are never auto-upgraded to streaming authorizations.
- The legacy API does not read or interpret new-version internal events.
- New DTOs must be independently validatable and compatibility-testable.

## ADR-10 — staged, module-scoped authorization

**AUTHORIZED THIS ROUND:**

1. Update and close out the PR #71 decision documentation.
2. Complete the required authorization records and documentation merge gates.
3. Create an independent WP1 branch.
4. Design and implement an isolated new versioned contract definition, pure contract
   validation, and compatibility tests.
5. Submit a WP1 PR after code-ownership and independent-review requirements are met.

WP1 may add **new** versioned interfaces/types under `pkg/contracts/**`, but must not change
the meaning, serialization, ID computation, signature binding, default execution path, or
production behaviour of the existing `m1a-v1` surface. Adding a versioned type must not be
used as cover to change an old public constant or legacy caller behaviour.

**STILL NOT AUTHORIZED:** WP2 production DB outbox, WP2b MySQL outbox, WP3 Runner/CLI
streaming production integration, WP4 anchor/seal/verifier, Issue #14 checkpoint/resume, and
any default-behaviour or existing M1 release change.

## ADR-11 — trusted local execution boundary

**APPROVED DIRECTION:** no independent protection against an active attacker is promised.

Trust assumptions: the current Runner is not maliciously modified; the local OS and database
are inside the trusted execution boundary; approval private keys are not leaked; the target
database is administered by an authorized operator; a subject with database write access is
not assumed to be detectable when rewriting audit records.

Guaranteeable goals: correct per-batch transaction outcome, data/receipt atomic consistency
under verified storage conditions, accurate partial/unknown classification, independent
Validate, and a bounded working set. Nothing beyond these is claimed.

## Consequences for the decision tables

- ADR-01..ADR-07, ADR-09, ADR-10, ADR-11 are **DECIDED** as direction above.
- ADR-08 stays **PENDING MEASUREMENT AND OWNER APPROVAL**.
- Implementation gates G1–G12 stay open: none is verified by this decision.
- WP1 is limited-authorized; WP2, WP2b, WP3, WP4 remain NOT AUTHORIZED / DEFERRED.
- Issue #13 remains OPEN; Issue #14 remains unstarted; M1 release and tag unchanged.
