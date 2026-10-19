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

Every module that MaskRiver compiles or links now has its verbatim licence text
under `licenses/`: the twelve `go.mod` requirements above, direct and indirect.
The texts are copied unmodified from the module cache of the pinned versions;
none was edited or summarised. `go.sum` also contains modules that are only
transitive build metadata and contribute no linked code; they are deliberately
not reproduced here.

### Additional texts embedded inside dependencies

Some dependencies ship further third-party material *inside* the module. Most of it
is compiled into the binary; a few entries below are shipped text with no linked
code, and the table says which is which. Because `modernc.org/libc` selects its musl-translated
implementation on linux/amd64 without a build tag, this material is linked into a
default build and is therefore a release condition, not a curiosity:

| Module | Additional text | File |
|---|---|---|
| `modernc.org/libc` | Go / musl / go-netdb / nixpkgs material | `licenses/modernc-libc-LICENSE-3RD-PARTY` |
| `modernc.org/sqlite` | bundled third-party notices | `licenses/modernc-sqlite-LICENSE-3RD-PARTY` |
| `modernc.org/sqlite` | SQLite itself (public domain) | `licenses/modernc-sqlite-LICENSE-SQLITE` |
| `modernc.org/sqlite` | sqlite-vec (shipped text; upstream list; not linked into a default build) | `licenses/modernc-sqlite-LICENSE-SQLITE_VEC` |
| `modernc.org/memory` | Go runtime material | `licenses/modernc-memory-LICENSE-GO` |
| `modernc.org/memory` | mmap-go material | `licenses/modernc-memory-LICENSE-MMAP-GO` |
| `modernc.org/memory` | project logo reference (URL) | `licenses/modernc-memory-LICENSE-LOGO` |
| `golang.org/x/sys` | patent grant | `licenses/x-sys-PATENTS` |

**Repository texts are not a compliant release artifact.** A binary release must
still be checked as its own deliverable: it needs a NOTICE-style collation of
every applicable text above, the complete licence texts shipped alongside the
artifact, and — for the MPL-2.0 covered `go-sql-driver/mysql` files — a stated way
to obtain the corresponding source form. This repository documents the material;
it does not package it.

Binary distribution must include the applicable notices and, for MPL-2.0
covered files, a way to obtain the corresponding source form. See
[database-drivers.md](database-drivers.md) for the per-dependency constraints.
