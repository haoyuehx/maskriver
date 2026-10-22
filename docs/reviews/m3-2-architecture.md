# M3-2 architecture review

- **Initial head:** `ed4152b`; recheck head: `1b428cb`; final code head reviewed for security: `0f3c380`.
- **Context:** independent read-only subagent (`opencode-go/deepseek-v4.1-flash`).
- **Verdict:** PASS after fixes; no P0/P1.

Initial P1s:

1. Textual DSN comparison let aliases of one MySQL database pass as source and
   target. Fixed with a live `@@server_uuid` + `DATABASE()` identity check after
   both readers open and before any plan/write; the CI E2E covers a socket-path
   alias. Remaining documented limits: cloned datadirs can duplicate UUIDs
   (fail closed) and primary/replica pairs are not treated as one snapshot.
2. MySQL FK target schemas stayed physical, so differently named source/target
   databases could never pass describe. Fixed by mapping same-database FK
   targets to the shared logical schema; the E2E now plans a child table with an
   outgoing FK. The adapter test asserts the logical FK target.

Recheck PASS at `1b428cb`; `pkg/contracts` unchanged. Accepted P2: digest is not
an atomic consistent snapshot and cost scales with planned table size; the
denylist is not an exhaustive session-variable policy; some probe branches are
covered by CI rather than local tests.
