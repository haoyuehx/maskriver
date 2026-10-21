# Streaming and Recovery Guide

Streaming (`m2-stream-v1`) and local recovery (`m2-recovery-v1`) are Linux amd64
SQLite features.

```sh
maskriver init-outbox  --target target.db
maskriver init-recovery --target target.db
maskriver approve --streaming ... --trust-file ... --history-dir ...
maskriver mask --streaming --apply --approved --budget-file budget.json \
  --receipt-page-size 8 --run-id <32-hex> ... --key-file ...
maskriver status --target target.db --run-id <32-hex>
maskriver mask --streaming --apply --resume --run-id <32-hex> ... --key-file ...
```

Recovery supports only pure deterministic strategies. It verifies the committed
prefix by recomputation and the uncommitted suffix against the source; target
conflicts, wrong keys, source drift and commit-unknown states fail closed.
`status` is read-only. A SIGKILL can leave a hot rollback journal: `status`
reports `reason=hot_journal` (exit 3), and an explicit apply/resume recovers it
once before continuing. `validation_failed` is terminal for resume.

MySQL/Windows/macOS/WAL recovery is not supported.
