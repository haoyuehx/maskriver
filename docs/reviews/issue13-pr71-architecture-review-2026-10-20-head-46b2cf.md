# PR #71 Independent Architecture Review — head 46b2cf1

- **Reviewed SHA:** `46b2cf1127905eeeda6d3b2c4a953e85d6980f87`, independently verified from worktree Git `HEAD`; watchdog found no uncommitted changes at start/end.
- **Scope:** all five changed documentation files and supplied committed diffs `9d6e036..46b2cf1` and `c44a7c3..46b2cf1`; related current contracts, Runner/Worker Pool, DB transaction/path handling, approval binding, CLI summary, verifier and regression-test source.
- **Verdict:** **P0=0, P1=1, P2=4; DESIGN BLOCKED; Merge verdict BLOCK.** Design-coherence only. No commands/tests, GitHub/CI query, DB/anchor probe, crash/fault injection or native platform test.

## P1 — completion classification permits an INCOMPLETE seal to be reported complete

**Location:** ADR:88,102–109 and failure matrix row 9 (reviewed head).

ADR allows a terminal `COMPLETE` or `INCOMPLETE` seal, but `COMPLETE_VALID` only requires a “valid terminal seal”. Thus an anchored `INCOMPLETE` seal with otherwise valid events/counts can satisfy both `COMPLETE_VALID` and `INCOMPLETE_VALID`, contradicting exactly-one outcome and risking false success. Missing seal also overlaps: `INCOMPLETE_VALID` permits no terminal seal while `TRUNCATED_OR_MISSING` handles missing seal expected by independent evidence; row 9 uses an inconsistent `TAMPERED` status for a missing anchored tail.

**Minimum fix:** require `seal.outcome == COMPLETE` in `COMPLETE_VALID`; define precedence using independent catalog evidence (expected missing record => `TRUNCATED_OR_MISSING`; legitimate valid prefix/no expected completion => `INCOMPLETE_VALID`; altered digest => `TAMPERED_OR_UNTRUSTED`; unavailable/unanchored local evidence => `UNKNOWN`). Use exact status names in the failure matrix and add a planned case proving valid INCOMPLETE seal never completes.

## P2 findings

1. **Per-batch prepare is outside the sequence diagram loop** (`Design:118–123`). With multiple batches, later batches lack distinct prepare/anchor acknowledgments. Keep RunEnvelope outside; move each batch's prepare and acknowledgment inside loop before that batch's transaction.
2. **Blanket batch-ID uniqueness shorthand** (ADR:140) conflicts with a prepare and committed event sharing a batch ID, despite explicit event-scoped rules in ADR:80. Replace with reference to role-scoped uniqueness; specify append-only grants only for audit rows.
3. **Implementation approval checklist omits ADR-11** (`Design:198`); make ADR-01..11 and trust assumptions explicit prerequisites.
4. **Review history self-contradicts** (`review-history:40–43,56`): says c44 architecture review completed and later says it remains pending. Identify pending review as the post-correction head and preserve c44 provenance.

## Correct / bounded claims

- Append-only model now separates event ID and batch ID; rewrite test assumes already enrolled/anchored run; stale sink/API revision work packages were removed.
- A3 is only a feasible candidate: current WriteTx lacks outbox API and current Runner has no implementation. SQLite/MySQL durability and authoritative-primary requirements are unverified.
- AC S1/S2/S3 preserves global Issue wording; no streaming-only or heap-only narrowing is assumed. Legacy `[]BatchReceipt` remains Θ(K); other resource terms require caps.
- ADR-11's independent enrollment/catalog and per-batch live attestation meaningfully address first-anchor/deleted-known-run risk within stated trust assumptions. No production/AC readiness follows.

## Original Issue #13 AC provenance

No verbatim issue body/edit history is in supplied repository. Main provided active Issue #13 body fetched live on 2026-10-20, including exact functional text: “Concurrency is bounded and cancellable; ordering/partitioning is deterministic; memory growth is bounded by batch × workers.” The reviewer could not independently fetch GitHub. This review does not grant a streaming-only or application-heap-only exception and does not assert acceptance passed.

Full original reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/2365767b-efcd-4b1c-b22e-46e7837f306f_reviewer_output.md` (run ID `2365767b-efcd-4b1c-b22e-46e7837f306f`).
