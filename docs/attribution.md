# MaskRiver Attribution and Provenance

MaskRiver is an independently developed project. It is **not** a fork of any
other repository and shares no Git history with one.

## Code, data and test provenance

No source code, rule dictionary, configuration file, test fixture, credential or
test suite has been copied into this repository from another project. The Go
implementation, contracts, synthetic fixtures, CLI skeleton and tests under this
repository were written for MaskRiver.

## Dependency licences

MaskRiver itself is MIT licensed (see [`../LICENSE`](../LICENSE)). Third-party
dependencies keep their own licences and their verbatim texts are kept under
[`licenses/`](licenses):

| Dependency | Licence | Text |
|---|---|---|
| `modernc.org/sqlite` | BSD-3-Clause | `licenses/modernc-sqlite-LICENSE` |
| `modernc.org/libc` | BSD-3-Clause | `licenses/modernc-libc-LICENSE` |
| `modernc.org/mathutil` | BSD-3-Clause | `licenses/modernc-mathutil-LICENSE` |
| `modernc.org/memory` | BSD-3-Clause | `licenses/modernc-memory-LICENSE` |
| `github.com/go-sql-driver/mysql` | MPL-2.0 | `licenses/go-sql-driver-mysql-LICENSE` |
| `filippo.io/edwards25519` | BSD-3-Clause | `licenses/edwards25519-LICENSE` |
| `github.com/dustin/go-humanize` | MIT | `licenses/go-humanize-LICENSE` |
| `github.com/google/uuid` | BSD-3-Clause | `licenses/google-uuid-LICENSE` |
| `github.com/mattn/go-isatty` | MIT | `licenses/go-isatty-LICENSE` |
| `github.com/ncruces/go-strftime` | MIT | `licenses/go-strftime-LICENSE` |
| `github.com/remyoudompheng/bigfft` | BSD-3-Clause | `licenses/bigfft-LICENSE` |
| `golang.org/x/sys` | BSD-3-Clause | `licenses/x-sys-LICENSE` |

Every module in `go.mod` and `go.sum` now has its verbatim licence text under
`licenses/`. The texts are copied unmodified from the module cache of the pinned
versions; none was edited or summarised.

Binary distribution must include the applicable notices and, for MPL-2.0
covered files, a way to obtain the corresponding source form. See
[database-drivers.md](database-drivers.md) for the per-dependency constraints.
