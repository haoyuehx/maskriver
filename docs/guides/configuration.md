# Configuration Reference

The CLI has no config file that can enable writes. Write intent is only the
explicit `--apply` flag plus `--approved` and a valid decision.

| Flag | Meaning |
|---|---|
| `--source`, `--target` | existing absolute SQLite files (currently the only CLI dialect) |
| `--scope table.column` | repeatable ordered masking scope |
| `--strategy`, `--key-id`, `--key-file` | deterministic strategy and external key |
| `--min-samples`, `--sample-limit`, `--min-ratio` | detection evidence |
| `--batch-size`, `--workers` | bounded execution |
| `--trust-file`, `--history-dir` | deployment trust and local decisions |
| `--approved`, `--apply` | declaration + explicit write intent; neither alone authorizes |
| `--streaming`, `--budget-file`, `--receipt-page-size`, `--run-id` | M2 path |
| `--resume` | explicit recovery of an existing run |
| `--strict` | full-scan validation with adapter integrity checks |
| `--timeout` | operation deadline |

Environment variables do not enable writes. Secrets are read from files and are
never printed. See the security model and error reference.
