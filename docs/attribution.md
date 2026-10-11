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
| `github.com/go-sql-driver/mysql` | MPL-2.0 | `licenses/go-sql-driver-mysql-LICENSE` |

Binary distribution must include the applicable notices and, for MPL-2.0
covered files, a way to obtain the corresponding source form. See
[database-drivers.md](database-drivers.md) for the per-dependency constraints.
