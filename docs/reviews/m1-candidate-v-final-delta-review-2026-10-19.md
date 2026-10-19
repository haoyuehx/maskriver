# M1 final candidate delta review — `maskriver-m1-linux-amd64-f4aeff9.tar.gz`

- Date: 2026-10-19
- Reviewer role: independent delta reviewer (built none of the reviewed packages, did not write the builder fix)
- Artifact under review: `/tmp/final/maskriver-m1-linux-amd64-f4aeff9.tar.gz`
- Claimed SHA-256: `cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c`
- Claimed build commit: `f4aeff9103ad064f57f63846cf47de587bdabeea` (repo HEAD in this worktree; `git status --porcelain` empty)
- Builder used: committed `scripts/build-release.sh` at `f4aeff9`, SHA-256 `ccaa1998b504cc21b5d5927513473bde57b59f5fe40a970f32e0e19efdf8e964` (identical to `git show f4aeff9:scripts/build-release.sh`)
- Environment: Linux x86-64, `go1.26.3 linux/amd64`, `GOMODCACHE=/home/haoyue/.gvm/pkgsets/go1.26.3/global/pkg/mod`, `/bin/sh` → dash, default `umask 0002`
- Independence: `docs/releases/m1-candidate-linux-amd64-1f8e013.md` and `docs/reviews/m1-candidate-artifact-review-2026-10-19.md` were read only for context; no verdict or measurement in this report is inherited from them. Every number below comes from a command executed in this session.
- Scratch directories (all outside the repo, created empty for this review): `/tmp/rev-v1`, `/tmp/rev-final`, `/tmp/repro`, `/tmp/repro-cold`, `/tmp/repro-u022`, `/tmp/repro-v1`, `/tmp/smoke`

## 1. Measured digests

| Artifact | Claimed | Measured (`sha256sum`) | Result |
|---|---|---|---|
| `/tmp/final/maskriver-m1-linux-amd64-f4aeff9.tar.gz` | `cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c` | `cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c` | **match** |
| `/tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz` (v1) | `e50ec59744547888d6143f71538319020807dddad7431e70727df9d708ef8b6f` | `662bd601e1eaeb343705810518d883f604bde906ea0986af382fd4ee3a082de0` | **mismatch** — see P2-1 |

The file under the v1 name is therefore **not** the archive v1's recorded digest describes; no copy of `e50ec597…` exists on this machine (`find /tmp -name 'maskriver*.tar.gz'` yields only `662bd601…` for that name plus the final candidate and scratch rebuilds). Its payload is a **dirty-tree** build (`go version -m`: `v0.0.0-20261019025025-47c6e3c…+dirty`, `vcs.modified=true`, `BUILD-INFO.md` commit `47c6e3c`, licences mode `0444`). The `0444` licence mode and the dirty VCS stamp are exactly the v2 defect signature. **v2 remains not reviewable** and is recorded here as unreproducible: it required a manual `chmod 444 docs/licenses/*` that no committed script performs, so it cannot be reproduced from committed state.

## 2. Reproducibility — decisive check

Fresh clone, no manual step, no working-tree edit, no `chmod`:

```console
$ git clone -q /home/haoyue/Project/maskriver /tmp/repro/src
$ cd /tmp/repro/src && git checkout -q f4aeff9103ad064f57f63846cf47de587bdabeea
$ git status --porcelain          # empty
$ ./scripts/build-release.sh /tmp/repro/maskriver-m1-linux-amd64-f4aeff9
building 0.1.0-dev (f4aeff9) for linux/amd64
… package contents: 25 entries …
$ cd /tmp/repro && tar --sort=name --owner=0 --group=0 --numeric-owner \
      --mtime='2026-10-19 00:00Z' \
      -czf maskriver-m1-linux-amd64-f4aeff9.tar.gz maskriver-m1-linux-amd64-f4aeff9
$ sha256sum maskriver-m1-linux-amd64-f4aeff9.tar.gz
cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c  maskriver-m1-linux-amd64-f4aeff9.tar.gz
```

**Byte-identical to the shipped archive**, including every mode and mode-derived tar field. Reproduced a second time with a cold Go build cache (`GOCACHE=/tmp/repro-cold/gocache`): same digest `cf06a7e4…`. The committed script pins `0644` on `LICENSE NOTICE.md MPL-2.0-SOURCE.md BUILD-INFO.md` and `licenses/*`, `0755` on `maskriver`, so no manual `chmod` is needed. **Reproducibility: YES.**

Two boundary facts (not failures of the decisive check, but recorded because the claim is unconditional):

- The archive is **umask-sensitive**. Rebuilding the same commit and the same tar command under `umask 0022` gives `10e652d0…` (all 25 file **contents** byte-identical; only directory entries, `MANIFEST` and `SHA256SUMS` are `0755`/`0644` instead of `0775`/`0664`). See P2-2.
- The v1 build cannot be reproduced from its documented commit: clean `1f8e013` → `1b7a0420…`, 4 505 188 B (claimed `e50ec597…`, 4 505 193 B); `47c6e3c` → `0d7e654c…`; the f4aeff9 script run on a dirty `1f8e013` tree → `6c586aaa…`. None matches. See P2-1.

## 3. Delta table

Both archives extracted into fresh directories (`/tmp/rev-v1/maskriver-m1-linux-amd64-1f8e013`, `/tmp/rev-final/maskriver-m1-linux-amd64-f4aeff9`). File set is identical: **25 payload files** + `MANIFEST` + `SHA256SUMS` = 27 regular files and 2 directories = 29 tar entries in both; no symlink, hardlink, device or FIFO entry in either.

`MANIFEST` is byte-identical between the two archives (`71aabe48…`); set comparison both directions is exact in both artifacts (0 missing, 0 unlisted beyond `MANIFEST`/`SHA256SUMS` themselves).

| File | v1 SHA-256 | final SHA-256 | State |
|---|---|---|---|
| `BUILD-INFO.md` | `2c63820c432d5772b58e1916f86c05415a9baf3c2d366709686471ce690f34d6` | `33780ee54b1801df88318aa0aaec5297fc23cd7f8f1171d8f41de86322cbe41f` | **differs** (commit line only) |
| `LICENSE` | `a9e0129b9227d192c70e23e775a80a2b0967898c848a8fd6b2a30083990935d2` | same | identical |
| `MPL-2.0-SOURCE.md` | `5892cc08da197a8e6e970772f76293c7396dc3e7a192993335f8f7e02734e0ad` | same | identical |
| `NOTICE.md` | `7d28334a879cfe1f337092b4109c26b58b37d6472c56a0024ef26c1b67552176` | same | identical |
| `licenses/bigfft-LICENSE` | `dd26a7abddd02e2d0aba97805b31f248ef7835d9e10da289b22e3b8ab78b324d` | same | identical |
| `licenses/edwards25519-LICENSE` | `2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067` | same | identical |
| `licenses/go-humanize-LICENSE` | `a973b4498c13eb74baa2a8e5c351426a6826f2fcdd909916dbe53ee2e755fd71` | same | identical |
| `licenses/go-isatty-LICENSE` | `08eab1118c80885fa1fa6a6dd7303f65a379fcb3733e063d20d1bbc2c76e6fa1` | same | identical |
| `licenses/go-sql-driver-mysql-LICENSE` | `fab3dd6bdab226f1c08630b1dd917e11fcb4ec5e1e020e2c16f83a0a13863e85` | same | identical |
| `licenses/go-strftime-LICENSE` | `38ae43959daf953a393a585b2988672cb65a5a541aca0d0be5e72595a0a16883` | same | identical |
| `licenses/google-uuid-LICENSE` | `0a8d61ed3cbfd5312326e8126c31ce9c627a283adc99131b56896d29ada04b2d` | same | identical |
| `licenses/modernc-libc-LICENSE` | `95ff867eb55a56935fa7492406cfa953fb7c13ca73f4c0a86ae05756b4605600` | same | identical |
| `licenses/modernc-libc-LICENSE-3RD-PARTY` | `f597097efe3d97021f89170746bd3a0fb9a8b6fb26b82043ed68a4e0283bee6c` | same | identical |
| `licenses/modernc-mathutil-LICENSE` | `bfa9bf72a72ca009fd62a8f84fca3dca67e51d93af96352723646599898b6cf5` | same | identical |
| `licenses/modernc-memory-LICENSE` | `59895e669f48f168b6b858358f6005779cdf40a265f7828813061b56af67b496` | same | identical |
| `licenses/modernc-memory-LICENSE-GO` | `2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067` | same | identical |
| `licenses/modernc-memory-LICENSE-LOGO` | `5ae5bee3072a841376451b48d8cfcec7188e10543926d5870828d36c8a750dc5` | same | identical |
| `licenses/modernc-memory-LICENSE-MMAP-GO` | `c2eba69f20d05414538c3a5df7694dde392e065ff70882e1625e90f5d6659fff` | same | identical |
| `licenses/modernc-sqlite-LICENSE` | `c6fe05491a60ae13bcd223088d2705e36dede24e5587226231d2459ada5c4822` | same | identical |
| `licenses/modernc-sqlite-LICENSE-3RD-PARTY` | `b4366b9c27a9364633e015011a231913ed19bf1228f335bf617e50a6f7b168a6` | same | identical |
| `licenses/modernc-sqlite-LICENSE-SQLITE` | `8438c9c89b849131ead81d5435cb97fcf052df5b0b286dda8a2d4c29e6cb3fd0` | same | identical |
| `licenses/modernc-sqlite-LICENSE-SQLITE_VEC` | `6ce72bbe12d975bd5286e5ab0a064c069693300c47bccbc57bec18485f1621ea` | same | identical |
| `licenses/x-sys-LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` | same | identical |
| `licenses/x-sys-PATENTS` | `96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc` | same | identical |
| `maskriver` | `bc553aabf70b26febe7fd527ad231d357b9ff3a37eac8f94ba590eb8e94f2345` | `a088ecd3a38c6751b885bbfc851c58c0fb693c7c8be83ebbe62bdd72f8cb658f` | **differs** |

`SHA256SUMS` differs only in the `BUILD-INFO.md` and `maskriver` lines, which follows from the two content differences above. The binary differs, so **v1's smoke result is not cited**; a fresh smoke was run against this binary (§6).

Because the file under the v1 name already carries the final `NOTICE.md` (byte-identical), the diff of the two **archived** NOTICE files is **empty**. The meaningful NOTICE delta is against the documented v1 commit: a clean rebuild of `1f8e013` produces the static NOTICE (`782d8827…`, 12 linked rows) and the full diff to the final file is:

```diff
@@ Linked into the binary @@
+Derived from `go list -deps` for the target platform, not from a static list, so
+the index can only claim what the build actually links.
 | Component | Licence | Text in this package |
 |---|---|---|
-| modernc.org/sqlite | BSD-3-Clause | licenses/modernc-sqlite-LICENSE |
-| modernc.org/libc | BSD-3-Clause | licenses/modernc-libc-LICENSE |
-| modernc.org/mathutil | BSD-3-Clause | licenses/modernc-mathutil-LICENSE |
-| modernc.org/memory | BSD-3-Clause | licenses/modernc-memory-LICENSE |
-| github.com/go-sql-driver/mysql (MPL-2.0) | MPL-2.0 | licenses/go-sql-driver-mysql-LICENSE |
 | filippo.io/edwards25519 | BSD-3-Clause | licenses/edwards25519-LICENSE |
 | github.com/dustin/go-humanize | MIT | licenses/go-humanize-LICENSE |
 | github.com/google/uuid | BSD-3-Clause | licenses/google-uuid-LICENSE |
-| github.com/mattn/go-isatty | MIT | licenses/go-isatty-LICENSE |
-| github.com/ncruces/go-strftime | MIT | licenses/go-strftime-LICENSE |
+| github.com/go-sql-driver/mysql | MPL-2.0 | licenses/go-sql-driver-mysql-LICENSE |
 | github.com/remyoudompheng/bigfft | BSD-3-Clause | licenses/bigfft-LICENSE |
 | golang.org/x/sys | BSD-3-Clause | licenses/x-sys-LICENSE |
+| modernc.org/libc | BSD-3-Clause | licenses/modernc-libc-LICENSE |
+| modernc.org/mathutil | BSD-3-Clause | licenses/modernc-mathutil-LICENSE |
+| modernc.org/memory | BSD-3-Clause | licenses/modernc-memory-LICENSE |
+| modernc.org/sqlite | BSD-3-Clause | licenses/modernc-sqlite-LICENSE |
@@ Further texts @@
-## Shipped upstream but carrying no linked code
+## Shipped upstream but not linked by this build
 | modernc.org/sqlite | sqlite-vec | licenses/modernc-sqlite-LICENSE-SQLITE_VEC |
 | modernc.org/memory | project logo reference (URL) | licenses/modernc-memory-LICENSE-LOGO |
+| github.com/mattn/go-isatty | module requirement, not present in this build's link set | licenses/go-isatty-LICENSE |
+| github.com/ncruces/go-strftime | module requirement, not present in this build's link set | licenses/go-strftime-LICENSE |
```

That diff is the whole of the NOTICE change: the false linked-set claim is removed, the two unlinked MIT texts are re-labelled, and the table is now derived at build time.

Modes, owner, mtime and tar metadata:

| Property | v1 archive | final archive |
|---|---|---|
| `maskriver` | `0775` | `0755` (pinned) |
| `LICENSE`, `NOTICE.md`, `MPL-2.0-SOURCE.md`, `BUILD-INFO.md` | `0664` | `0644` (pinned) |
| `licenses/*` (20) | `0444` (manual chmod) | `0644` (pinned) |
| `MANIFEST`, `SHA256SUMS` | `0664` | `0664` (umask-derived, not pinned — P2-2) |
| directories | `0775` | `0775` (umask-derived) |
| owner/group in tar | numeric `0/0` | numeric `0/0` |
| mtime in tar | `2026-10-19 08:00` (+0800 = `2026-10-19T00:00:00Z`) on every entry | same, uniform |
| entry order | name-sorted | name-sorted (byte order, verified against `LC_ALL=C sort`) |
| pax/extended headers | none | none |
| gzip header | `1f8b`, method 8, **flags 0 (no FNAME/FCOMMENT/FEXTRA)**, mtime 0, OS 3 | identical |

## 4. NOTICE correctness against reality

Link set, measured on the target platform from a clean clone of `f4aeff9`:

```console
$ CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./cmd/maskriver
filippo.io/edwards25519 · filippo.io/edwards25519/field · github.com/dustin/go-humanize
github.com/google/uuid · github.com/go-sql-driver/mysql · github.com/remyoudompheng/bigfft
golang.org/x/sys/unix · modernc.org/{libc, libc/sys/types, libc/uuid/uuid, mathutil,
memory, sqlite, sqlite/lib, sqlite/vtab}      # 15 third-party import paths
$ … | go list -f '{{.Module.Path}}'  → 10 third-party modules
$ go version -m maskriver | grep -c '^	dep'      → 10   (same 10 modules)
$ for s in mattn/go-isatty ncruces/go-strftime; do strings -a maskriver | grep -c "$s"; done
0
0
$ grep -c UNMAPPED NOTICE.md      → 0
```

| Check | Result |
|---|---|
| NOTICE "Linked into the binary" rows vs `go version -m` deps | **exact set match, 10/10**: edwards25519, go-humanize, uuid, go-sql-driver/mysql, bigfft, x/sys, libc, mathutil, memory, sqlite |
| `mattn/go-isatty` claimed as linked? | **no** — it is in "Shipped upstream but not linked by this build"; absent from `go list -deps`, 0 `strings` hits |
| `ncruces/go-strftime` claimed as linked? | **no** — same group, same evidence |
| `UNMAPPED` entries | **0** |
| Every linked module maps to a text present in the package | 10/10 (`test -f` per path) |
| Every `licenses/…` reference in NOTICE resolves in the extracted package | 20/20 (the 21st regex hit is the prose sentence ending `licenses/go-sql-driver-mysql-LICENSE.` — same file) |
| Shipped texts not referenced by NOTICE | none (20 files, 20 referenced) |
| Licence labels match the shipped texts | verified by header: MPL-2.0 text starts "Mozilla Public License Version 2.0"; MIT texts (humanize, isatty, strftime, sqlite-vec) carry the MIT grant; the rest carry the 3-clause BSD grant |

Grouping accuracy, checked against the selected build files rather than taken on trust:

- **Musl material is genuinely compiled** on linux/amd64: `go list -f '{{.GoFiles}}' modernc.org/libc` selects `libc_musl.go`, `libc_musl_linux_amd64.go`, `etc_musl.go`, `mem_musl.go`, `native_musl.go`, `pthread_musl.go`, …, and `licenses/modernc-libc-LICENSE-3RD-PARTY` really contains the `Go`, `musl libc`, `go-netdb` and `NixOS/nixpkgs` sections.
- **`modernc.org/memory` mmap-go material is compiled**: `mmap_unix.go` (`//go:build unix`) is in `GoFiles` for this target and its header points at `LICENSE-MMAP-GO`.
- **sqlite-vec is not linked**: `modernc.org/sqlite/lib` compiles only `sqlite_linux_amd64.go`; `strings` finds 0 occurrences of `vec0`, `sqlite_vec`, `sqlite-vec`, `sqlite3_vec_init`, and upstream's own `LICENSE-3RD-PARTY` states sqlite-vec is linked only when `modernc.org/sqlite/vec` is imported. The "not linked" grouping is accurate.
- Minor precision issue in the "Further texts" caption — see P2-4.

## 5. Artifact integrity

| Check | Command | Result |
|---|---|---|
| Per-file digests | `sha256sum -c SHA256SUMS` in the extraction | **25/25 OK, exit 0**, zero failures |
| `SHA256SUMS` self-reference | `grep -c SHA256SUMS SHA256SUMS` | 0; it lists exactly the 25 `MANIFEST` entries |
| MANIFEST both directions | set comparison, `os.walk` vs `MANIFEST` | 25/25; **0 missing, 0 unlisted** (only `MANIFEST`/`SHA256SUMS` are outside it, by construction) |
| No truncated licence | last byte of all 20 texts is `\n`; sizes equal to origin | 20/20 |
| Texts byte-identical to `<GOMODCACHE>` at the pinned versions | `cmp` against each module's own file | **20/20 identical, 0 fail** |
| Texts byte-identical to `docs/licenses/` at `f4aeff9` | `git show f4aeff9:docs/licenses/<n> \| cmp -` | **20/20 identical** |
| Duplicate texts among the 20 | one digest appears twice: `2d36597f…` = `edwards25519-LICENSE` and `modernc-memory-LICENSE-GO` | both are the same Go Authors BSD-3 text, shipped by two modules; legitimate |
| MPL-2.0 source obtainable, version matches | `go mod download github.com/go-sql-driver/mysql@v1.10.1` → exit 0; module dir present; `LICENSE` starts "Mozilla Public License Version 2.0" | version `v1.10.1` = `go version -m` dep and `go.mod` and `MPL-2.0-SOURCE.md` |
| Residue in archive | names matching `*.db|*.sqlite|*.key|*.pem|*.log|*.json|decision|.git|.linked` | none; builder's `$OUT/.linked` temporary is removed before `MANIFEST` |
| Host paths / PII in archive | `grep -rnE '/home/|/Users/|C:\\Users|PRIVATE KEY|…'` | no build-host path; only upstream licence copyright lines (`mattn.jp@gmail.com`, `edsrzf@gmail.com`) |
| Host paths / keys in binary | `strings` for `/home/`, `/Users/`, `/root/`, `/var/lib/`, `go/pkg/mod`, `.cache/go-build`, `GOMODCACHE` | 0 hits; the only `/tmp/` string is the driver default `/tmp/mysql.sock`. `BEGIN … PRIVATE KEY` occurrences: 0. `password` hits are mysql-driver option names |
| Binary form | `file`, `ldd` | ELF 64-bit x86-64, **statically linked**, stripped; `ldd` → "not a dynamic executable" |
| Embedded build metadata | `go version -m` | `-trimpath=true`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, `vcs.revision=f4aeff9103ad…`, `vcs.modified=false` |
| BUILD-INFO vs reality | compare to `go version -m` / `git rev-parse` | version `0.1.0-dev`, commit `f4aeff9…`, `go1.26.3 linux/amd64`, build mode — all consistent; the binary is a clean (`+` no `dirty`) build |
| Startup | `./maskriver --version` / `--help` / `scan` (no args) | exit 0 (`MaskRiver 0.1.0-dev`) / 0 / **2** |

## 6. Smoke — run because the binary differs from v1

All steps are separate processes; synthetic values only (`user{1,2,3}@example.invalid`), source and target are independent file copies (`people(id INTEGER PRIMARY KEY, email TEXT NOT NULL, full_name TEXT)`). The CLI requires an explicit scope (`--scope people.email`); without it the commands exit 2 as invalid arguments.

| Step (separate process) | Exit | Output |
|---|---|---|
| `scan --scope people.email --min-samples 3` | **0** | `scan: complete=true columns=1 issues=0` |
| `mask --strategy fake_email` (default dry run) | **0** | `preview: dry-run=true rows=3 complete=true (no row values shown)`; target SHA-256 unchanged `b4afacfb…`; history dir empty |
| `approve` (Ed25519, trust file + reviewer key) | **0** | `approve: stored=true scopes=1 valid_for=24h0m0s`; one `<plan-id>.decision` (922 B) |
| **refusal A**: `mask --apply` without `--approved` | **2** | sanitized refusal, no write |
| **refusal B**: `mask --approved --apply` with an empty decision store | **1** | `apply: complete=false committed_batches=0 committed_rows=0`; target SHA-256 unchanged `b4afacfb…` |
| `mask --approved --apply` (new process) | **0** | `apply: complete=true committed_batches=1 committed_rows=3 outcome_unknown=false`; target → `72839804…` |
| `validate --approved --strict` (new process) | **0** | `validate: pass=true tables=1 rows=3 issues=0` |

Source SHA-256 stayed `b4afacfb…` throughout; no `-wal`/`-shm`/`-journal` sidecars appeared; masked target values are synthetic (`…@maskriver.local`). The multi-table claim in `BUILD-INFO.md` was **not** re-run by me (single-table path only) and is therefore UNVERIFIED in this record.

## 7. Findings

**P0: none.**
**P1: none attaching to the final candidate.** The repro build is byte-identical, the link set is exact with zero `UNMAPPED`, every licence text is present and unmodified, and no sensitive material is in the archive or binary.

- **P2-1 — the v1 record cannot be re-derived from any artifact present.** `/tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz` measures `662bd601…`, not the recorded `e50ec597…`; its payload is a dirty build of `47c6e3c` (`+dirty`, `vcs.modified=true`) with `0444` licence modes — the v2 signature, i.e. the non-reproducible build. Clean rebuilds of the documented v1 commit give `1b7a0420…`, so the v1 archive is unreproducible with what is on disk. Rated P2 **only because** v1 is superseded and the final candidate's bytes are independently verified end-to-end here; if v1 were still the ship candidate this would be P1. Recommended: withdraw or annotate the v1 digest claim in `docs/releases/…1f8e013.md`, and keep `e50ec597…` out of any release note until an archive with that digest is re-produced.
- **P2-2 — the reproducibility claim is umask-sensitive.** The builder pins modes for the executable, documents and `licenses/*`, but not for `MANIFEST`, `SHA256SUMS` or the two directories. Under `umask 0022` the same inputs produce `10e652d0…`: contents identical, directory/`MANIFEST`/`SHA256SUMS` modes `0755`/`0644` instead of `0775`/`0664`. The documented command sequence does not state a umask. Fix: `chmod 0644 MANIFEST SHA256SUMS` and `chmod 0755 . licenses` in the builder (or document `umask 002`).
- **P2-3 — the link-set query is fail-open.** `scripts/build-release.sh` runs `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go list -deps … | grep … | sort > .linked` under `set -eu` **without `pipefail`** (dash: `set -eu; false | sort; echo $?` → 0). A failing `go list` would silently produce an empty `.linked`, an empty "Linked into the binary" table, and a *successful* build. The shipped artifact is unaffected (10 correct rows, 0 `UNMAPPED`), and the licence texts are still copied wholesale, but the index could regress without a build failure. Fix: `set -o pipefail` where supported, or stage `go list` output to a file and check its exit status before the pipeline.
- **P2-4 — "Further texts … Required because the material is compiled in" over-states one row.** `modernc.org/memory`'s six compiled files carry only "The Memory Authors" (`LICENSE`) and "Evan Shaw" (`LICENSE-MMAP-GO`) headers; the only file in the module mentioning Go Authors is `LICENSE-GO` itself. Shipping it is defensible (it is a text the dependency ships), and no attribution obligation is missed, but the caption should not be read as every row being compiled-in. Same for `modernc-sqlite-LICENSE-3RD-PARTY`, which is a bundle that itself catalogues non-linked components.

Observations without severity: `f4aeff9` still contains only `docs/releases/m1-candidate-linux-amd64-1f8e013.md`; the final candidate's digest `cf06a7e4…` is not recorded in the repository, so the claim lives only in the release task. The extracted binary and all texts are otherwise self-consistent. The old unarchived scratch directory `/tmp/rel/maskriver-m1-linux-amd64` (a `4ddd5e7` `+dirty` build) is not part of any delivery.

Final candidate verdict: V2 RELEASE CANDIDATE READY — this artifact (commit `f4aeff9`, reproduced byte-for-byte from a fresh clone with no manual step) supersedes the non-reproducible v2 and the unavailable v1 archive.
