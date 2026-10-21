# SQLite Tutorial

Use two distinct existing files; the source is opened read-only and the target is
written only by an explicit `--apply` run with a valid Ed25519 decision.

```sh
maskriver scan --source /work/source.db --target /work/target.db \
  --min-samples 20 --scope people.email
maskriver mask --source /work/source.db --target /work/target.db \
  --strategy fake_email --key-id demo --key-file /work/key.bin --scope people.email
maskriver approve ... --reviewer-id ... --trust-file ... --history-dir ... --reviewer-key-id ... --reviewer-key-file ...
maskriver mask ... --approved --apply
maskriver validate ... --strict
```

SQLite streaming/recovery additionally requires Linux, rollback journal, and
explicit `init-outbox`/`init-recovery`; see the streaming/recovery guide. WAL is
refused by the source snapshot gate.
