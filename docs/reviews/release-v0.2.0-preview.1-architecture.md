# v0.2.0-preview.1 — independent architecture review

- **Reviewer context:** fresh read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Reviewed candidate:** `7266a540f41e544954316ae87024108a9b2d5775` (then-current main).
- **Verdict:** **CHANGES REQUIRED** — code/architecture PASS, documentation/provenance P1 only.

## P1

1. README/roadmap claimed the release was already published while no tag/release
   existed, and the candidate archive was an untagged pseudo-version build.
2. The cited release record lacked the commit/tag/archive SHA/size and
   post-download re-verification promised by README/roadmap/attribution.

## P2

- Stale `internal/runner/streaming.go` comment said recovery was not implemented.
- Release smoke harness was not committed at the reviewed SHA.
- Tag-bound provenance is not exercised by CI (`--allow-untagged` in CI only).
- The runtime guard is OS-only; linux/arm64 is untested but not refused.

## Verified PASS

Frozen contracts and `pkg/contracts` unchanged; M1 workflow unchanged; Issue #13
streaming/outbox/budget unchanged for non-recovery runs; Issue #14 state machine,
strategy allowlist, journal/synchronous/FK gates and linux identity match the
protocol; support boundary and non-claims consistent; package/archive structure,
reproducibility, checksums and binary provenance verified.

## Disposition

- P1-1/P1-2: documentation now distinguishes the release candidate from the
  published tag/asset and records the candidate commit/SHA; final tag and
  post-download SHA are recorded in the GitHub Release acceptance comment.
- P2-1/P2-2: stale comment updated; release smoke committed.
- P2-3: final tag-bound build/verify is a documented release step.
- P2-4: documentation states only linux/amd64 is built and tested; other Linux
  architectures are not refused and not supported.

## Second-context recheck

A fresh independent recheck at `43f411c`/`82d8948` confirmed the prior P1
documentation fixes were substantively present (publication wording, candidate
archive binding, deferred final SHA) and that the hot-journal/busy-timeout code
delta does not change the recovery state machine or transaction atomicity. The
remaining recheck finding was a record-only issue: this review file still said
CHANGES REQUIRED while the acceptance table said PASS. This section and the
updated acceptance table record the closure; no code P0/P1 remains.

**Recheck verdict: PASS** (record-only corrections applied).
