# PR #71 Independent Security Design Review — 2026-10-20

- **Reviewed head:** `c44a7c38785b07cfe4e5f18d0e85c2ba7a282c9a` (SHA supplied by Main, reviewer could not independently run git).
- **Scope:** A3/outbox candidate and current contracts/runner/db/history/config/CLI source; read-only. Read AGENTS, architecture, test-plan, contracts, feature matrix, ADR, design and review history. No diff artifact, commands, tests, DB probe, CI query, crash/fault injection, or implementation verification. This is a design review only; historical CI is not evidence for this head.
- **Verdict:** **P0=0, P1=1, P2=2; Merge verdict: BLOCK.**

## Previous four Security P1 dispositions

| Finding | Classification on reviewed head | Evidence / limits |
|---|---|---|
| P1-1 Commit→durable receipt crash window | **RESOLVED at design level only** | ADR requires business updates and COMMITTED event in same target DB/transaction (ADR §A3 and state table). This changes the atomic boundary; it is not post-Commit append. Existing `internal/runner/execute.go:715–735` still appends after Commit; A3 is not implemented or tested. |
| P1-2 Envelope/Seal/hash chain/trust anchor | **PARTIALLY RESOLVED — remains OPEN** | ADR defines envelope, chain, seal, external anchor and limitations, but first-anchor trust, attacker-controlled initial finalization and discovery of wholly deleted/unknown runs are not closed. Details below. |
| P1-3 arbitrary Consumer block/cancellation | **RESOLVED at design level only** | ADR removes callback/channel; synchronous internal outbox Q=0; no detached goroutine or hard-kill promise. Runtime cancellation/resource release remain unimplemented/unverified. |
| P1-4 independent sink path safety | **PARTIALLY RESOLVED — residual launch gate** | No separate receipt path; DB transaction errors cover short write/space failures. Target DB file/path identity, permissions, platform open semantics and durability checks remain requirements, not an executable or tested capability check. Existing `Stat` is not safe-open proof. |

## P1 — First-anchor trust and whole-run discovery are underspecified

**Locations:** ADR §“Hash chain, signature and trust anchor” and §“Verification outcomes” (current ADR lines 85–106, 183–187 at reviewed head).

**Trigger:** Threat model includes an actor able to modify target DB records before the run's first trusted anchor confirmation, or delete/replace a whole run; audit discovery relies only on RunIDs read from that mutable target DB.

**Risk:** Anchor provides unique RunID, idempotent same-digest handling, conflict rejection, signed response and authenticated lookup. But an attacker can rewrite all local records before the first anchor submission, after which a legitimate finalizer may anchor the rewritten digest as the first accepted value. Conflict handling does not protect first submission. Lookup of a known RunID cannot detect a whole run deleted when its identifier is no longer known. Therefore current wording cannot promise integrity against these cases.

**Minimum disposition:** Either (a) explicitly narrow the threat guarantee to tampering detection **after a trusted first anchor for a known RunID**, and explicitly exclude pre-anchor malicious rewrite and unknown-run deletion; or (b) design an independent trusted run-registration/catalog and authorized commitment protocol, including protection against rewriting a run before its initial commitment. Do not treat “anchor is trusted” or hash chaining as sufficient. If broader threat protection is required, add corresponding verification and fault-injection evidence.

**Status:** P1 OPEN; original P1-2 not closed.

## P2 findings

1. **Durability/path conditions are not directly executable preflight yet** (ADR §A3 and filesystem boundary). Exact SQLite sync level/init/ongoing check; MySQL durability setting/primary identity/drift; and platform-specific directory ACL/open-file identity are not fixed. Existing `internal/db/db.go:176–195` stats then opens by path; `internal/config/runtime.go:71–82` `SameFile` is only a point-in-time check. Require a fail-closed supported storage/platform matrix and never advertise resistance to host root.
2. **Old design text still offers the superseded Sink path** (`docs/design/issue13-bounded-receipts-design.md` §12–§13): caller-supplied Sink, synchronous Sink work package and optional later outbox conflict with current outbox-only candidate. Mark/remove stale path so implementers cannot ship a downgrade.

## Other conclusions

- A3 correctly requires SQLite same `main`/`sql.Tx`, and MySQL same primary transaction with transactional tables such as InnoDB; neither has implementation or DB-probe evidence. Frozen `WriteTx` exposes Update/Commit/Rollback only, so outbox needs a deliberate contract/DB owner proposal.
- CommitUnknown treatment is conservative and consistent with `internal/db/write.go:246–261`: no blind retry; absent/unavailable evidence stays unknown.
- Anchor availability failure must produce incomplete/unknown, never business retry. Anchor durability/monotonicity and key custody need independent operational guarantees; an HSM name alone is not proof.
- Pre-provisioned outbox/no Apply DDL is reasonable; schema digest, Validate/scanner exclusion and reporting of reserved extra tables remain design gates.
- New audit policy/outbox identity must be bound to the approved operation; current history binds plan/snapshot/target/schema/config/policy/scopes (`internal/history/record.go:254–285`). Do not let a runtime option bypass existing approvals.
- New envelope/outbox operations must remain behind explicit write authorization and all approval/preflight/dry-run gates; Preview must remain non-persistent. No automatic data replay or #14 checkpoint/resume is authorized.
- No PII/value/key/DSN/private key in records; table names and stable identifiers may still disclose metadata/relationships.

**Merge/implementation status:** BLOCK. No Owner decision, implementation permission, production test evidence, or AC pass is implied. Full original output artifact: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/b2077420-4368-40b1-ba8f-7028c47e9e15_go-reviewer_output.md` (run ID `b2077420-4368-40b1-ba8f-7028c47e9e15`).
