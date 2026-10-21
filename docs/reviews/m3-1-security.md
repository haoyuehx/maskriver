# M3-1 security review

- **Head:** `b61e76b` (merged as `cc178aa`)
- **Context:** independent read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Verdict:** PASS after fixes; no P0/P1.

Prior P1s: SQLite FK check was outgoing-only (parent could dangle) -> now
database-wide `PRAGMA foreign_key_check`; MySQL `CHECK TABLE` parsing was
truncated/unverified -> MySQL strict now fails closed with `ErrUnsupported`.
No SQL identifier interpolation was introduced; non-strict behavior is
unchanged; Writer non-NULL->NULL refusal and approval skew are correct.

Accepted P2: MySQL strict is not implemented and remains UNVERIFIED; full DB
quick_check cost; local trust/key boundary is not RBAC.
