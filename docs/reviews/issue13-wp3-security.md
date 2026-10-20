# Issue #13 WP3 — independent Security review record

**Scope:** PR #74, read-only. **Reviewer context:** independent AI reviewer, not the implementer.
**Reviewed heads:** `24c1b49d994137434fc3f520580d433e92389856`, `af5ebf4d5d692671967e5638bfd172318f25f70a`
(base `97e95c8be712ad12491bdfe7ada1cea41f85eec7`).
**Limits:** no tests, DB probes or GitHub operations run by the reviewer.

## Verdicts

| Head | P0 | P1 | P2 | Verdict |
|---|---|---|---|---|
| `24c1b49` | 0 | 0 | 3 | OK with notes |
| `af5ebf4` | 0 | 0 | 0 | OK with notes |

## Findings and dispositions

1. **Unreadable ambiguity candidate skipped.** The streaming ambiguity scan ignored read errors and
   could accept a canonical record while a second unreadable record might claim the same plan.
   Fixed: non-`NotExist` read failures return a sanitized `ErrUnsafe`.
2. **Cancellation not propagated in history enumeration.** Both ambiguity checks used
   `context.Background()`. Fixed: the request context flows through directory walking.
3. **Audit-incomplete counted as unknown commit.** `BatchAuditIncomplete` incremented
   `BatchesUnknown`. Fixed: it now maps to `RunStateAuditIncomplete` and a failed batch count, while
   genuine commit uncertainty still maps to `commit_unknown`.

## Confirmed at the reviewed code

- Real Ed25519 streaming records with a separate trusted-key/expiry/revocation path; legacy records
  cannot be upgraded and streaming records cannot authorize the legacy path.
- Canonical content digest blocks Scope/NormalizationVersion/type/order drift under the same
  `Plan.ID`.
- All approval refusals happen before the outbox is opened or a transaction exists; nil verifier
  refuses; preview writes nothing.
- Commit errors are `ErrCommitUnknown`, never retried, never success; receipt row counts must match.
- Apply performs no DDL; `init-outbox` is a separate explicit command.
- No values, replacements, keys, DSNs or paths in summaries/errors; reserved outbox tables refused.
- G5/G7/G9 limits are documented and not overstated.

## Residual risks

- File-handle-level TOCTOU binding remains unverified (trusted-local-directory boundary).
- Receipt-time ENOSPC, fsync failure and kill during commit remain unverified.
- Windows/MySQL streaming runtime unsupported/unverified; documented as such.
