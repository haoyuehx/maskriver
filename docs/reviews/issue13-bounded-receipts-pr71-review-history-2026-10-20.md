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

## Current design review gate

The above reviews are **historical** and were not performed on the updated A3/outbox ADR. New independent Architecture Review and Security Review must record their exact current final head, full file scope, tests/commands actually run, P0/P1/P2, limits and each original P1 disposition. Until then the four P1s remain OPEN for the current proposal. If provider failure occurs, record it; use another independent reviewer only transparently and do not convert failure to PASS. Issue Owner decisions remain pending.
