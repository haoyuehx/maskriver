# M3-2 security review

- **Heads:** initial `ed4152b`, recheck `1b428cb`, final `0f3c380`.
- **Context:** independent read-only subagent (`opencode-go/deepseek-v4.1-flash`).
- **Verdict:** PASS on final code; no P0/P1.

Initial review found no P1. P2: arbitrary DSN params could disable reader
read-only/FK/session safety or inject SET clauses via comma values and scope
aliases. Fixed by:

- a single shared `internal/db/mysqlparam` policy that normalizes `@@`, `@@session.`,
  `@@local.`, `@@global.`, `SESSION ` and `GLOBAL `, rejects non-identifier keys,
  comma/semicolon values, and safety variables including `unique_checks`;
- live post-connect checks: readers require `transaction_read_only=1`, writers
  require `foreign_key_checks=1` and `unique_checks=1`;
- FK target logical mapping, alias identity refusal, credential-free
  `TargetIdentity`, identifier quoting and CI positive-PASS enforcement were
  re-verified.

Accepted P2: live checks sample the pooled connection (safe at MaxOpenConns=1);
`charset`/`collation` are driver-level SET NAMES surfaces not covered by the
param policy; the denylist is not exhaustive for non-safety sysvars.
