# MySQL Tutorial (8.0.46)

MySQL support covers the bounded M1 path only:
Scan -> Preview -> separate Approve -> Apply -> Validate. Use two distinct,
disposable databases on an isolated server. Never point both sides at the same
database or at production.

```sh
export SRC='<user>:<password>@tcp(<host>:3306)/maskriver_source?timeout=5s'
export DST='<user>:<password>@tcp(<host>:3306)/maskriver_target?timeout=5s'

maskriver scan --dialect mysql --source "$SRC" --target "$DST" \
  --scope people.born --scope notes.note

maskriver mask --dialect mysql --source "$SRC" --target "$DST" \
  --scope people.born --scope notes.note \
  --strategy cn_date --key-id demo --key-file /secure/key.bin

maskriver approve --dialect mysql --source "$SRC" --target "$DST" \
  --scope people.born --scope notes.note \
  --strategy cn_date --key-id demo \
  --trust-file trust.json --history-dir history \
  --reviewer-key-id reviewer-1 --reviewer-key-file reviewer.key \
  --reviewer-id reviewer@example.invalid

maskriver mask --dialect mysql --source "$SRC" --target "$DST" \
  --scope people.born --scope notes.note \
  --strategy cn_date --key-id demo --key-file /secure/key.bin \
  --trust-file trust.json --history-dir history --approved --apply

maskriver validate --dialect mysql --source "$SRC" --target "$DST" \
  --scope people.born --scope notes.note \
  --strategy cn_date --key-id demo --key-file /secure/key.bin \
  --approved --strict
```

## Gates and limits

- The same physical database is refused after connecting, including unix/tcp,
  localhost/IP and path aliases.
- DSNs that can disable read-only/FK/session/unique safety or inject SET
  clauses are refused, including `@@session.`/`GLOBAL ` aliases; the live
  reader re-checks read-only and the writer re-checks FK/unique checks.
  Credentials are not printed, but they are visible in the process command line;
  protect the host and shell.
- Plan binds the full content of every planned table. Cost grows with planned
  table size; unplanned tables are not hashed. The source must be isolated and
  quiescent because the digest is not an atomic consistent snapshot.
- Strict validation checks `CHECK TABLE ... QUICK` status rows and orphan rows
  for visible FKs touching the planned table. It does not verify CHECK
  expressions, duplicate UNIQUE values, unplanned tables, replication, or
  invisible cross-schema FKs.
- MySQL streaming, outbox, checkpoint and recovery are **not supported** and are
  refused. `m2-stream-v1`/`m2-recovery-v1` remain SQLite-only.

`notes.note` above must contain date-formatted text for `cn_date`; use a
column-appropriate strategy for other text. The acceptance evidence is `TestMySQLCLIWorkflow` in the pinned, network-disabled
MySQL 8.0.46 CI container. Adapter-only tests are not CLI acceptance.
