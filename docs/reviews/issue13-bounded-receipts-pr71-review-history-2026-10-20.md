# PR #71 — bounded-receipt design review history

This file preserves review summaries and their exact head provenance in the repository. It does not replace the original reviewer artifacts, does not claim the latest design was reviewed unless its SHA is listed, and is not an Owner approval.

## Review of initial proposal

### Architecture review 1
- Reviewer: independent `reviewer` context; read-only.
- Reviewed head: `ee3b445daba30d03bfcdedb8e7d8807a2b945793`.
- Scope: design proposal and current contracts/runner/DB/history/verify/CLI/performance source seams.
- Verdict: **P1=2, P2=3**. P1: readiness verdict preceded unresolved AC/audit choices; approval `os.ReadDir` introduced O(H) memory. P2: K formula/global table boundary; reader lookahead backing arrays; phase-5 sampler grows with duration.
- Disposition: document was changed to DESIGN BLOCKED, added O(H), per-table K and lookahead/sampler accounting. Evidence transcript saved in the Pi review artifact store under `17e7e09b-f52b-468f-a395-323dfa524b5a_reviewer_output.md` (session/run ID `17e7e09b-f52b-468f-a395-323dfa524b5a`).

### Architecture follow-up
- Reviewer: independent `reviewer` context; read-only.
- Reviewed head: `ea89ecc8d45cf0e2f32a02da3f9f350b89b2f585`.
- Scope: corrected document and relevant current source.
- Verdict: **P0=0, P1=0, P2=4**. Follow-up findings: global K bound needed per-table summation; candidate budget needed H; impossibility proof should rely on the concrete slice representation, not independent information; Commit errors before the underlying Commit attempt are not unknown.
- Disposition: per-table bound, H term, representation-based proof and pre-attempt distinction were added. Report stored as `4890d6ce-6a15-4957-8c43-44c114555639_reviewer_output.md` (session/run ID `4890d6ce-6a15-4957-8c43-44c114555639`).

### Architecture review 2
- Reviewer: independent `reviewer` context; read-only.
- Reviewed head: `97d781ebf8a32cc644d5a3658b2fe0cf497ea2b8` (the reviewer verified this SHA from its isolated worktree).
- Scope: design and source seams; no tests/commands/network.
- Verdict: **P0=0, P1=0, P2=2**. One finding alleged H was absent from the formula; the reviewed document did contain `A(H,filenameBytes)`, so that item was not supported by the source text. The other was a valid inaccurate count of the first review's P2 items, later corrected.
- Report stored as `97ddb842-806b-409c-a272-80d0580bbc2e_reviewer_output.md` (session/run ID `97ddb842-806b-409c-a272-80d0580bbc2e`).

## Security review of previous design candidate

- First `go-reviewer` invocation at the earlier proposal failed with provider `Insufficient Balance`; it produced **no review verdict** and is not counted as a completed review.
- An independent read-only `reviewer` context then examined `97d781ebf8a32cc644d5a3658b2fe0cf497ea2b8`; its verdict was **P0=0, P1=4, P2=1 — BLOCK candidate D production implementation**.
- P1-1: post-Commit/pre-durable-receipt crash, cancellation, ENOSPC and acknowledgment ambiguity.
- P1-2: no defined run envelope/terminal seal/trust anchor; hash chain alone cannot detect full rewrite or tail truncation.
- P1-3: arbitrary synchronous callbacks may ignore cancellation/block/re-enter; bounded queue does not guarantee bounded exit.
- P1-4: separate sink path lacked alias/symlink/no-clobber/permission/write/fsync/disk-full protocol.
- P2: global `APIRevision` bump can invalidate old Plan validation and change Plan.ID/approval binding.
- Scope/limits: read design and relevant contracts/runner/DB/history/CLI/tests; no code execution, no tests, no external Issue/PR verification. Original transcript stored as `ff88ff9e-4c4e-4e8e-9a98-cf06cd9c6c8a_reviewer_output.md` (session/run ID `ff88ff9e-4c4e-4e8e-9a98-cf06cd9c6c8a`).

## Architecture Review — A3/outbox proposal at c44a7c3

- Reviewer: independent `reviewer` context; read-only.
- Reviewed head: `c44a7c38785b07cfe4e5f18d0e85c2ba7a282c9a`, verified via worktree `.git`/`HEAD`; no worktree changes at review time.
- Verdict: **P0=0, P1=0, P2=3**; proposed `DESIGN READY — PENDING OWNER APPROVAL` for that design head only. Exact review and limits: [`issue13-pr71-architecture-review-2026-10-20.md`](issue13-pr71-architecture-review-2026-10-20.md); original transcript `eb3ffd95-119b-4422-aa19-858295b23d5b_reviewer_output.md` (run ID `eb3ffd95-119b-4422-aa19-858295b23d5b`).
- P2s: event-row update versus append-only/grants ambiguity; whole-chain rewrite test lacked a previously anchored precondition; old design §12–§13 still had sink/API guidance inconsistent with ADR. These are corrected in later documentation changes and must be rechecked on the new head.
- Limitations: no diff artifact; no CI, tests, DB probes or GitHub fetch. Original AC excerpt was supplied by Main; full original Issue was not independently verified.

## Security Review — A3/outbox proposal at c44a7c3

- Reviewer: independent `go-reviewer` context; read-only.
- Reviewed head: `c44a7c38785b07cfe4e5f18d0e85c2ba7a282c9a` (Main supplied the SHA; reviewer could not run git); no diff artifact and no tests/DB probes/CI query.
- Verdict: **P0=0, P1=1, P2=2; BLOCK**. Full formal summary: [`issue13-pr71-security-review-2026-10-20.md`](issue13-pr71-security-review-2026-10-20.md); original output artifact `b2077420-4368-40b1-ba8f-7028c47e9e15_go-reviewer_output.md` (run ID `b2077420-4368-40b1-ba8f-7028c47e9e15`).
- Historical P1-1 Commit→receipt gap: RESOLVED at design level only by same target transaction outbox; implementation unverified. P1-2 trust anchor: PARTIALLY RESOLVED, remains OPEN because first-anchor authenticity and full-run discovery/deletion guarantees are underspecified. P1-3 arbitrary Consumer: RESOLVED at design level only by eliminating callback/channel; runtime unverified. P1-4 separate sink path: PARTIALLY RESOLVED by removing it; target DB path identity/permissions/durability checks remain launch gates.
- New P1: an attacker able to modify DB before first anchor can rewrite records and submit a rewritten digest as the first anchor; a known-RunID lookup cannot discover a wholly deleted run if IDs are only read from mutable DB. The design must narrow its claim to post-first-anchor tampering for known IDs, or define independently trusted run enrollment/catalog/commit authorization. Do not treat P1-2 as closed.
- P2: DB durability/path conditions are not yet executable preflight; old design §12–§13 still described a caller-supplied Sink and optional later outbox. The latter stale text has been removed in a subsequent documentation commit; current-head review needed for that edit.

## Follow-up reviews — A3/trust-enrollment proposal at 46b2cf1

- Independent Architecture Review verified exact head `46b2cf1127905eeeda6d3b2c4a953e85d6980f87` and inspected both supplied diffs plus current source. Verdict **P0=0, P1=1, P2=4; BLOCK**. P1: an explicit INCOMPLETE seal could still satisfy COMPLETE_VALID; missing-seal classifications conflict. P2s: per-batch BatchPrepare outside loop in diagram; blanket batch-ID uniqueness shorthand; implementation checklist omitted ADR-11; this review history incorrectly said the completed c44 review was pending. Formal summary: [`issue13-pr71-architecture-review-2026-10-20-head-46b2cf.md`](issue13-pr71-architecture-review-2026-10-20-head-46b2cf.md); original artifact `2365767b-efcd-4b1c-b22e-46e7837f306f_reviewer_output.md` (run ID `2365767b-efcd-4b1c-b22e-46e7837f306f`).
- Independent Security Review received supplied diff and current docs/source at `46b2cf1127905eeeda6d3b2c4a953e85d6980f87` (SHA not independently verified). Verdict **P0=0, P1=0, P2=3**. Prior P1-1/2/3 resolved only at design level (P1-2 within ADR-11 scope); P1-4 partly resolved because target path controls remain unimplemented. P2s: platform/durability preflight not executable, BatchPrepare outside loop, implementation checklist omitted ADR-11. Formal summary: [`issue13-pr71-security-review-2026-10-20-head-46b2cf.md`](issue13-pr71-security-review-2026-10-20-head-46b2cf.md); original artifact `544ea5bb-509e-4dc5-a527-2175d8485e56_go-reviewer_output.md` (run ID `544ea5bb-509e-4dc5-a527-2175d8485e56`).

The above two reviews both concern the exact 46b2cf1 head, **not** later corrections. Current-head CI on 46b2cf1 passed Ubuntu/Windows/isolated-MySQL; these are docs-PR checks, not outbox implementation evidence.

## Final-head follow-up at e1faa58 (fallback model)

- Initial fresh Architecture run `b66eb0fc-64c8-46f1-b0cc-a935f2196f8f` and Security run `937417d1-6bdb-4c35-9ff2-d06585814ed5` failed with provider usage limit and produced no verdict. They are not reviews and do not count toward gates.
- Per task authorization, independent contexts were rerun using `openai-codex/gpt-6-luna` after the `gpt-6-astra` usage limit. Architecture reviewer: supplied SHA `e1faa582ca5fa2d07478f081418c968101049f97`, found **P0=0/P1=0/P2=1**, failure-matrix Runner/verifier status ambiguity; exact summary [`issue13-pr71-architecture-review-2026-10-20-head-e1faa58.md`](issue13-pr71-architecture-review-2026-10-20-head-e1faa58.md), original artifact `1fa9df07-f643-4ba1-a7ee-ced25b6177a6_reviewer_output.md` (run ID `1fa9df07-f643-4ba1-a7ee-ced25b6177a6`). Reviewer checked abbreviated checkout SHA and clean state but could not inspect diff artifacts.
- Security reviewer: Main-supplied SHA `e1faa582ca5fa2d07478f081418c968101049f97`, could not independently verify Git or inspect `/tmp` diff; found **P0=0/P1=0/P2=1**, durability/platform/path preflight remains an implementation gate. Exact summary [`issue13-pr71-security-review-2026-10-20-head-e1faa58.md`](issue13-pr71-security-review-2026-10-20-head-e1faa58.md), original artifact `28be4513-7402-4fde-a246-93e02b1afcc6_go-reviewer_output.md` (run ID `28be4513-7402-4fde-a246-93e02b1afcc6`).

Both e1 reviews confirm design-level P1 dispositions only; no production implementation, durability evidence, or Issue #13 AC pass. Security P2 remains a required future implementation gate.

## Follow-up reviews at 49434f3

- Independent Architecture Review: **P0=0/P1=0/P2=1**. The `COMMIT_ATTEMPTED` row's verifier result said “UNKNOWN if queried before authoritative resolution,” contradicting the stated after-stop/readback-only rule. Formal report [`issue13-pr71-architecture-review-2026-10-20-head-49434f3.md`](issue13-pr71-architecture-review-2026-10-20-head-49434f3.md); original artifact `270e21a6-869a-4916-85b3-820919bda728_reviewer_output.md` (run ID `270e21a6-869a-4916-85b3-820919bda728`).
- Independent Security Review: **P0=0/P1=0/P2=1**. Target DB path identity, permissions and durability/backend matrix remain an unproven implementation gate. Formal report [`issue13-pr71-security-review-2026-10-20-head-49434f3.md`](issue13-pr71-security-review-2026-10-20-head-49434f3.md); original artifact `72a66e80-350f-4d8a-8865-cfe5caf17e5b_go-reviewer_output.md` (run ID `72a66e80-350f-4d8a-8865-cfe5caf17e5b`).
- Both report the supplied SHA/base and docs-only scope; the full diff was unavailable to their repo-only review tools. No tests, outbox implementation or DB/anchor probes were run.

## Follow-up reviews at 240ce83

- Independent Architecture Review confirmed the 494 table ambiguity is corrected, found **P0=0/P1=0/P2=1**, and recommended `DESIGN READY — PENDING OWNER APPROVAL`; its P2 was stale DESIGN BLOCKED/final-review-pending status text. Formal summary: [`issue13-pr71-architecture-review-2026-10-20-head-240ce83.md`](issue13-pr71-architecture-review-2026-10-20-head-240ce83.md); original artifact `081696fe-467d-4751-9969-1abc71a9c6d1_reviewer_output.md` (run ID `081696fe-467d-4751-9969-1abc71a9c6d1`).
- Independent Security Review found **P0=0/P1=0/P2=1**, retaining target path/durability support as an unimplemented gate; no Security P1 remains open at design level within ADR-11’s unapproved threat scope. Formal summary: [`issue13-pr71-security-review-2026-10-20-head-240ce83.md`](issue13-pr71-security-review-2026-10-20-head-240ce83.md); original artifact `acdf15a5-8f39-4b18-b786-52d789e459e3_go-reviewer_output.md` (run ID `acdf15a5-8f39-4b18-b786-52d789e459e3`).
- Architecture verified exact SHA/clean state; Security received Main-supplied SHA and could not access full diff. Neither ran tests/DB/anchor probes or validated implementation.

## Follow-up reviews at 45f754d

- Independent Architecture Review: **P0=0/P1=0/P2=1**. It confirmed the DESIGN READY status is properly limited, but found stale phrases saying a review is pending at `docs/design/issue13-bounded-receipts-design.md:3,245`. Formal report [`issue13-pr71-architecture-review-2026-10-20-head-45f754d.md`](issue13-pr71-architecture-review-2026-10-20-head-45f754d.md); artifact `0f6883fb-bed4-4a7c-a419-522ed82ee762_reviewer_output.md` (run ID `0f6883fb-bed4-4a7c-a419-522ed82ee762`).
- Independent Security Review: **P0=0/P1=0/P2=1**. Remaining issue is target path/backend/durability as an unimplemented gate; no design-level P1 remains within ADR-11 scope. Formal report [`issue13-pr71-security-review-2026-10-20-head-45f754d.md`](issue13-pr71-security-review-2026-10-20-head-45f754d.md); artifact `518239d3-ce5c-4372-b848-c12dc379ed2d_go-reviewer_output.md` (run ID `518239d3-ce5c-4372-b848-c12dc379ed2d`).
- Architecture verified exact supplied HEAD/clean worktree; Security received SHA from Main and could not independently verify Git. Both lacked the committed base diff; neither ran tests, DB/anchor probes or CI queries.

The design-document review-pending wording was removed before `717b17a`; independent Architecture and Security reviews on that head found this history sentence itself still stale. This paragraph records that finding historically; the wording is now in past tense. Their formal reports are linked in the following section. Future branch changes require their own exact-head review before merge consideration. Issue #13 stays OPEN, #14 is not started, and PR #71 remains unmerged.

## Follow-up reviews at 717b17a

- Independent Architecture Review: **P0=0/P1=0/P2=1**, stale present-tense sentence in this review-history file. Formal report [`issue13-pr71-architecture-review-2026-10-20-head-717b17a.md`](issue13-pr71-architecture-review-2026-10-20-head-717b17a.md); artifact `a608db0d-55d2-4321-8f8b-bad2043e4b8f_reviewer_output.md` (run ID `a608db0d-55d2-4321-8f8b-bad2043e4b8f`).
- Independent Security Review: **P0=0/P1=0/P2=1**, same stale history wording; target path/durability remains an implementation gate, not a P1. Formal report [`issue13-pr71-security-review-2026-10-20-head-717b17a.md`](issue13-pr71-security-review-2026-10-20-head-717b17a.md); artifact `aa0db902-7044-481d-bd0e-55c68ee0e9e1_go-reviewer_output.md` (run ID `aa0db902-7044-481d-bd0e-55c68ee0e9e1`).
- Architecture confirmed supplied HEAD/clean worktree; Security received SHA from Main. Both lacked full base diff; no tests or database/anchor probes were run.

## Owner-decision round — full diff audit and package reviews

- **Main full-diff audit (not an independent verdict):** complete `git diff 9d6e036..332cb80` inspected;
  19 added Markdown files, no code/contract/CI/release changes, no binaries/symlinks, `git diff --check`
  PASS, no credential patterns. Recorded in [`issue13-pr71-full-diff-review.md`](issue13-pr71-full-diff-review.md).
- **H1 independent full-diff review** at `332cb80fdb2dfccd718ee793bd87087e1c3e0af6`: read the supplied
  complete diff artifact, confirmed docs-only scope and honest AC/status claims. **P0=0/P1=0/P2=2**:
  (a) ADR-03 bundled an “independent witness” into A3 while the owner package separated them;
  (b) the package claimed a full-diff review without naming the audited head. Artifact
  `7be3c905-82fd-4976-bf1d-412e32c8528a_reviewer_output.md` (run ID `7be3c905-82fd-4976-bf1d-412e32c8528a`).
- **H2 independent Architecture review** of the owner package at `332cb80`: **P0=0/P1=0/P2=2**:
  B2 treated durable receipt/crash semantics as core AC even though the original AC is a memory bound;
  WP2 bundled SQLite and MySQL despite the ADR-07 staged recommendation. Artifact
  `fe29b4da-56f6-4c5f-ad31-caeb4acbb793_reviewer_output.md` (run ID `fe29b4da-56f6-4c5f-ad31-caeb4acbb793`).
- **H2 independent Security review** of the owner package at `332cb80`: **P0=0/P1=0/P2=1**: “receipt
  complete / full audit” wording could be read as tamper-evident or long-term retention although A3 is
  same-transaction consistency only. Artifact `531a9bb1-0f5e-404f-9d4e-d0b8c5807fb2_go-reviewer_output.md`
  (run ID `531a9bb1-0f5e-404f-9d4e-d0b8c5807fb2`).
- **Dispositions (all P2, documentation):** ADR-03/§A3 now scope A3 to same-transaction receipt atomicity
  and move anchor/seal to ADR-05; the package names the audited head and the audit gained an addendum;
  B2 marks durable-receipt semantics as conditional, not core; WP2 is SQLite-initial with a separately
  gated WP2b for MySQL; terminology now limits “receipt complete” to same-transaction consistency.
- All three reviewers were read-only and ran no tests or DB/anchor probes. Their exact-head re-review of
  the corrected head is required before any merge recommendation; none of these reviews authorizes
  implementation or Issue #13 acceptance.

## Convergence reviews at 7a95a7c / 4e35f3d

| Review | Head | Result | Findings / disposition |
|---|---|---|---|
| Full diff (H1) | `7a95a7c` | P0=0/P1=0/P2=2 | A3 row still said “durable receipt”; full-diff addendum claimed a review not yet recorded. Both fixed in `4e35f3d` (durability separated; addendum now says exact-head review is not claimed). Artifact `eb7eb1b2-df7e-4b1a-935d-a9fc0ca7c592_reviewer_output.md`. |
| Architecture (H2) | `7a95a7c` | P0=0/P1=0/P2=1 | SQLite platform/path gates G4–G5 sat in deferrable WP4. Fixed in `4e35f3d`: WP2 now requires G1–G9 including G4–G5 with fail-closed unsupported combinations; WP4 keeps only anchor-specific gates. Artifact `12dfc8e1-108b-4ba1-8ad5-f9a73d5dbf88_reviewer_output.md`. |
| Security (H2) | `7a95a7c` | P0=0/P1=0/P2=0 | Terminology, A3/anchor separation, gates and invariants verified; no new finding. Artifact `2fb2c4c5-2f1b-4072-bc6f-5bac34482baa_go-reviewer_output.md`. |
| Full diff (H1) | `4e35f3d` | P0=0/P1=0/P2=0 | Docs-only diff confirmed; both prior P2 fixes present; no overclaim. Artifact `71c87c6e-694d-4c39-99c9-f41fcedbf9f2_reviewer_output.md`. |
| Security (H2) | `4e35f3d` | P0=0/P1=0/P2=0 | Same-transaction wording, anchor scope, WP2 gates and write invariants confirmed. Artifact `f853c9ee-dcc3-478f-86cb-3c5225bb4125_go-reviewer_output.md`. |
| Architecture (H2) | `4e35f3d` | P0=0/P1=0/P2=1 | WP4 still listed MySQL gate G12. Fixed after this record: G12 moved to WP2b, WP4 keeps G10–G11. Artifact `e83021ef-38d0-4490-9bc4-a60ebf1bb8fd_reviewer_output.md`. |

All reviews were read-only; none ran tests, database/outbox/anchor/crash probes, or CI. They verify documentation
consistency only, not runtime behaviour, durability, or Issue #13 acceptance. This history section is itself a
later documentation-only commit; the exact-head CI on the final commit remains the CI gate.

## Owner direction round — 2026-10-20

**Decision record:** `docs/design/issue13-owner-direction-2026-10-20.md` records ADR-01..07/09/10/11
as decided direction (S2 scope revision, legacy O(K) retained, A3 same-DB transaction for the new
path, anchor deferred, Linux local SQLite first, parallel versioned contract, trusted local
boundary, WP1-limited authorization). ADR-08 remains pending measurement. Provenance is the
repository owner's delegated technical direction; it is **not** a GitHub signature.

**Repository records:** the Issue #13 acceptance-scope revision is appended to the issue body with the
original checklist preserved verbatim, and a pointer comment records the decision. Reviewers of this
round must re-verify the documentation at the exact new head; prior-head verdicts do not carry over.

**Implementation boundary:** only WP1 (new versioned contract types, pure validation, compatibility
tests under `pkg/contracts/**`) is authorized. WP2/WP2b/WP3/WP4 remain NOT AUTHORIZED/DEFERRED.
No outbox, anchor, durability or streaming runtime is implemented by this round.
