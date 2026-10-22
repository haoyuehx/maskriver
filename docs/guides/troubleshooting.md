# Troubleshooting

All failures are intentionally sanitized. The tool never prints row values,
primary keys, DSNs, passwords or key material. Use the exit code, the command and
the safe summary to decide what to check.

## Exit codes

- `0`: complete success.
- `1`: operation refused, incomplete, conflicting or validation failed.
- `2`: usage/configuration refusal (missing flags, invalid dialect, identical
  source/target DSN, unsafe DSN, bad budget). An alias of the same MySQL
  database is detected only after connecting and exits `1`.
- Conflicting evidence during SQLite streaming/resume can map to `4`, and
  validation failure to `9`; see [Error reference](errors.md).

## Common refusals

| Symptom | Check |
|---|---|
| `scan: complete=false` | `--min-samples`/`--min-ratio` too strict, too few distinct non-NULL values, or the column is genuinely UNKNOWN. Never treat UNKNOWN as safe. |
| `operation refused` on `mask --apply` | `--approved`, `--trust-file`, `--history-dir` and a stored decision bound to the exact plan are all required. |
| Plan/approval mismatch | Data or schema changed after approval, or the scope order/strategy/key changed. Re-scan and re-approve; do not reuse an old decision. |
| MySQL same-database refusal | The source and target DSNs resolve to the same `@@server_uuid` + `DATABASE()`, including unix/tcp or path aliases. Use a different database or server. |
| MySQL unsafe DSN | Remove `multiStatements`, `allowAllFiles`, `allowCleartextPasswords`, safety-disabling session variables and comma/semicolon parameter values. |
| MySQL strict validation fails | `CHECK TABLE ... QUICK` returned a non-OK row, `foreign_key_checks`/`unique_checks` is off, or an orphan FK visible to the account exists. |
| SQLite streaming/recovery refused | Streaming is Linux amd64 rollback-journal SQLite only; WAL, MySQL, Windows and macOS are unsupported. |
| `status` reports `hot_journal` | A previous process was interrupted. `status` remains read-only; an explicit apply/resume can recover once. |

## What not to do

Do not retry a commit-unknown outcome automatically, do not disable strict
validation to make a failure disappear, and do not point both sides at one
database. Do not paste a DSN or key into a bug report; provide the sanitized
summary and command shape only.
