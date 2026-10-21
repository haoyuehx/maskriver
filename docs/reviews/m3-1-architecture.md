# M3-1 architecture review

- **Head:** `b61e76b` (merged as `cc178aa`)
- **Context:** independent read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Verdict:** PASS after fixes; no P0/P1.

Initial review confirmed strict had a real fail-closed difference and no
`pkg/contracts` change, but required MySQL `CHECK TABLE` iteration and doc sync.
Disposition: MySQL strict was made `ErrUnsupported` instead of shipping an
unverified partial parser; SQLite FK checking is database-wide; docs/contracts
were synchronized. Recheck PASS.

Accepted P2: MySQL strict untested/fail-closed; whole-DB `quick_check` cost;
duplicated skew constant; strict RowLimit rule is API-only (CLI has no
`--row-limit`).
