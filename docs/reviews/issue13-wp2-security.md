# Issue #13 WP2 — independent Security review record

**Scope:** PR #73, read-only. **Reviewer context:** independent AI reviewer, not the implementer.
**Reviewed heads:** `a3708f865808a9dbcd955403e5e454dc7e6231f0`, `100f71d839169bf7592d7938c6325017b844fc17`,
`297c57b20df15f1e8facd00a321c75e18ca78d3a` (base `24877565e39c312c59a628cf0f054fe33dc79b8e`).
**Limits:** no tests, DB probes or GitHub operations were run by the reviewer; current-head CI is a
separate gate.

## Verdicts

| Head | P0 | P1 | P2 | Verdict |
|---|---|---|---|---|
| `a3708f8` | 0 | 1 | 2 | BLOCK |
| `100f71d` | 0 | 0 | 0 | OK (delta confirmation) |
| `297c57b` | 0 | 0 | 0 | OK (delta confirmation) |

## Findings and dispositions

1. **P1 — marker-only schema validation** (`internal/db/outbox.go`): weakened tables could be
   accepted. Fixed by exact column-shape verification on open and install (`100f71d`), plus
   hidden/generated-column rejection (`297c57b`).
2. **P2 — caller-supplied receipt row count** could misstate a committed batch. Fixed: `Commit`
   requires `receipt.Rows` to equal the rows actually updated, else rollback + `ErrConflict`.
3. **P2 — reserved outbox tables accepted as business targets**. Fixed: `BeginBatch` rejects
   `IsOutboxTable(plan.Table.Table)` with `ErrUnsafe`.

## Confirmed security properties at the final reviewed code

- Business UPDATE and receipt INSERT are in one transaction on one connection; commit errors return
  `ErrCommitUnknown` with no retry; no path reports success without a same-transaction receipt.
- Outbox is a separate explicit API; it does not modify `OpenWriter` or legacy write authorization,
  and is not wired into Runner/CLI.
- `InstallOutboxSchema` is an explicit migration; no apply-time DDL; equal version + exact shape is a
  no-op, anything else fails closed.
- Values are parameterized; identifiers come from validated metadata; outbox table names are fixed
  constants.
- `journal_mode=delete` is required and never rewritten; session `synchronous=FULL` is set and
  re-read; `foreign_keys` must be ON and `query_only` OFF.
- Receipts and the schema carry identifiers/counts/timestamps only — no values, keys, DSNs or
  absolute paths.

## Residual risks (documented, not P1)

- **TOCTOU:** modernc SQLite reopens by pathname, so preflight `Lstat` + identity recheck cannot bind
  the SQLite handle. Supported boundary is the ADR-11 trusted local directory; no anti-replacement
  claim is made.
- **Durability:** `Commit()==nil` plus session `FULL` do not prove power-loss durability;
  receipt-time ENOSPC and mid-commit process termination remain unverified.
