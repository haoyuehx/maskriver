# MaskRiver Attribution and Provenance

MaskRiver is an independently developed project. It is **not** a fork of any
other repository and shares no Git history with one.

## Code, data and test provenance

No source code, rule dictionary, configuration file, test fixture, credential or
test suite has been copied into this repository from another project. The Go
implementation, contracts, synthetic fixtures, CLI skeleton and tests under this
repository were written for MaskRiver.

Behavioural references (for example a default sampling threshold, or a
tri-state classification model) were used during early design exploration as
prior art for solving a well-known problem shape. Those are documented in this
project's own terms in `docs/architecture.md`, `docs/roadmap.md` and
`docs/feature-matrix.md`; they are not a compatibility target and no third-party
implementation is reproduced or ported.

## Retained licence text

Earlier design documents in this repository referenced the MIT-licensed Python
project `dbmask` (Copyright (c) 2026 Siyuan Feng) as prior art. That project's
verbatim MIT licence text is retained at
[`licenses/dbmask-MIT-LICENSE.txt`](licenses/dbmask-MIT-LICENSE.txt) so the
reference is not misrepresented and the upstream notice remains available.

If any future contribution does adapt third-party code, documentation or test
material, the contributor must retain the applicable copyright notice, licence
text and provenance note in this file before the contribution is merged. An
independent repository does not remove that obligation.

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
