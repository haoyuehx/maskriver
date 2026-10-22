# Issue #86 — MySQL CLI design (M3-2)

## Scope

The MySQL CLI implements the same bounded M1 workflow as SQLite:

`scan -> preview -> approve (separate process) -> apply -> validate`

The real acceptance test is `cmd/maskriver.TestMySQLCLIWorkflow`, executed by
`scripts/test_mysql_container.py` in the existing pinned, network-disabled
MySQL 8.0.46 container. It creates two disposable databases (source and target),
runs the real CLI entry point, and checks durable target state. Adapter-only
tests are not accepted as CLI evidence.

Not in scope: MySQL streaming/outbox, MySQL checkpoint/resume, cross-database
transactions, replication/lag handling, MySQL versions other than 8.0.46, and
production DSNs or personal data. MySQL recovery has no approved protocol and
is refused by the CLI.

## Configuration

- `--dialect mysql`, with `--source` and `--target` carrying DSNs.
- Both DSNs must name a database. The same server/network-address/database pair
  is refused even when the users differ. Different databases on one server are
  allowed.
- `multiStatements`, `allowAllFiles` and `allowCleartextPasswords` are refused.
- Credentials are never printed; refusals are sanitized.
- `--streaming`, `--resume` and the SQLite recovery/status commands are
  rejected for MySQL.

## Schema identity

A plan must be comparable across two differently named databases. The CLI uses
one logical schema (`main`) in every `TableRef`; each adapter maps that logical
name to its own physical database. Metadata queries use the physical name,
while reported foreign-key targets are mapped back to the logical name when the
target is in the same dataset.

## Source snapshot binding

SQLite binds `SourceSnapshotID` to the main database file's SHA-256. A MySQL
server has no single file with that meaning. For MySQL, Plan streams every row
of every planned table on the source, including all columns (not only masked
ones), and hashes a length-prefixed canonical encoding with SHA-256. This gives
a content identity at the time of planning.

Limits, stated explicitly:

- The cost is proportional to the planned tables' total size and is paid during
  Plan/approve/apply/validate, not only preview.
- It is not a database-wide snapshot: tables outside the declared plan are not
  hashed.
- SHA-256 collisions are assumed infeasible, but this is not a database
  isolation mechanism. The source must still be a quiescent, authorized copy or
  dedicated database; concurrent writers between planning steps can make the
  plan stale, in which case apply/validate fail closed on value/count drift.
- `TargetID` hashes a credential-stripped target DSN, so password rotation does
  not invalidate an approval and the target database identity is still bound.

## Write preflight and strict validation

The existing runner gates remain:

- identical source/target schemas with complete coverage;
- stable, safely ordered primary keys;
- no modified UNIQUE/CHECK/FK column and no trigger on either side;
- no incoming FK to a modified column;
- optimistic per-row expected-value updates and transaction batches.

For MySQL strict validation:

- every `CHECK TABLE ... QUICK` row must be `status`/`OK`; an empty result,
  warning, note or error fails closed;
- `@@SESSION.foreign_key_checks` must be 1;
- every foreign key touching the planned table (as child or parent) is checked
  with an explicit orphan query; any orphan or permission/query failure fails
  closed.

This validates the planned table's storage and all FKs touching it. It is not a
whole-server consistency check, MySQL replication verification, or a substitute
for application constraints.

## Evidence

- `TestMySQLCLIWorkflow` in the isolated MySQL CI job: Scan, dry-run Preview,
  refusal without approval, separate Approve, Apply, strict Validate, source
  unchanged, target changed, re-apply refusal, same-database refusal, streaming
  refusal.
- `TestMySQLAdapter`, `TestMySQLFixture` and the existing SQLite suites remain
  green to prove no SQLite regression.
