# M1 candidate artifact review — `maskriver-m1-linux-amd64-1f8e013.tar.gz`

- Date: 2026-10-19
- Reviewer role: independent release-artifact reviewer (built nothing in this package)
- Artifact: `/tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz`
- Claimed build commit: `1f8e0130f71a3273a19f10d23d1189241c902fff`
- Independence: the closure of Issue #11 and the verification claims in PR #62 / `docs/releases/m1-candidate-linux-amd64-1f8e013.md` were treated as unverified assertions. Every statement below comes from executing a command against the extracted bytes.
- Environment: Linux x86-64, `go1.26.3 linux/amd64`, `GOMODCACHE=/home/haoyue/.gvm/pkgsets/go1.26.3/global/pkg/mod`
- Working directories (empty at start, never the build tree): `/tmp/rev-fresh-50311/shipped`
  (extraction of the shipped tarball), `/tmp/rev-fresh-50311/rebuilt` (independent rebuild),
  `/tmp/rev-fresh-50311/smoke` and `/tmp/rev-fresh-50311/smoke/refuse` (synthetic fixtures).

## 1. Archive digest

```console
$ sha256sum /tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz
e50ec59744547888d6143f71538319020807dddad7431e70727df9d708ef8b6f  /tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz
```

Measured digest equals the expected `e50ec597…8ef8b6f`. PASS.

## 2. Extraction and per-file digests

```console
$ tar -xzf /tmp/rel/maskriver-m1-linux-amd64-1f8e013.tar.gz -C /tmp/rev-fresh-50311/shipped
$ cd .../maskriver-m1-linux-amd64-1f8e013 && sha256sum -c SHA256SUMS
… 25 × "OK" …
EXIT=0            # 25 entries in SHA256SUMS, 25/25 OK, zero failures
```

`SHA256SUMS` does not list `SHA256SUMS` (`grep -c 'SHA256SUMS\|MANIFEST' SHA256SUMS` → 0). It also does not list `MANIFEST`; both exclusions match the builder (`scripts/build-release.sh`: `sha256sum $(cat MANIFEST)` where MANIFEST is `find . -type f ! -name MANIFEST ! -name SHA256SUMS`). Self-reference absent ⇒ PASS. See P2-4 for the MANIFEST coverage note.

Tar entry audit: 29 entries, types `d` and `-` only; no symlinks, no hardlinks, no setuid/setgid bits (`tar -tvzf | awk '{print substr($1,1,1)}' | sort -u` → `-`, `d`). 27 regular files: 25 payload files + `MANIFEST` + `SHA256SUMS`.

## 3. MANIFEST vs. files actually present (both directions)

```console
$ python3  # set comparison, os.walk over the extraction root
manifest entries: 25 disk files: 27
missing (in manifest, not on disk): NONE
unlisted (on disk, not in manifest): ['MANIFEST', 'SHA256SUMS']

$ diff -u <(find . -type f ! -name MANIFEST ! -name SHA256SUMS | sed 's|^\./||' | sort) \
          <(sort MANIFEST)
EXIT=0
```

No missing entries; the only unlisted files are `MANIFEST` and `SHA256SUMS` themselves, which cannot list themselves. PASS.

## 4. Every NOTICE-referenced licence text (all 20, not a sample)

NOTICE references: 20 unique `licenses/...` paths. Existence: 20/20 present
(`test -f` per path). Byte-identity against the module origins actually pinned by `go.mod`
(`cmp`, all 20):

| Shipped file | Origin in `<GOMODCACHE>` | Result |
|---|---|---|
| licenses/bigfft-LICENSE | bigfft@v0.0.0-20230129092748-24d4a6f8daec/LICENSE | identical |
| licenses/edwards25519-LICENSE | filippo.io/edwards25519@v1.2.0/LICENSE | identical |
| licenses/go-humanize-LICENSE | go-humanize@v1.0.1/LICENSE | identical |
| licenses/go-isatty-LICENSE | go-isatty@v0.0.24/LICENSE | identical |
| licenses/go-sql-driver-mysql-LICENSE | go-sql-driver/mysql@v1.10.1/LICENSE | identical |
| licenses/go-strftime-LICENSE | go-strftime@v1.0.0/LICENSE | identical |
| licenses/google-uuid-LICENSE | google/uuid@v1.6.0/LICENSE | identical |
| licenses/modernc-libc-LICENSE | modernc.org/libc@v1.77.1/LICENSE | identical |
| licenses/modernc-libc-LICENSE-3RD-PARTY | modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md | identical |
| licenses/modernc-mathutil-LICENSE | modernc.org/mathutil@v1.7.1/LICENSE | identical |
| licenses/modernc-memory-LICENSE | modernc.org/memory@v1.12.1/LICENSE | identical |
| licenses/modernc-memory-LICENSE-GO | modernc.org/memory@v1.12.1/LICENSE-GO | identical |
| licenses/modernc-memory-LICENSE-LOGO | modernc.org/memory@v1.12.1/LICENSE-LOGO | identical |
| licenses/modernc-memory-LICENSE-MMAP-GO | modernc.org/memory@v1.12.1/LICENSE-MMAP-GO | identical |
| licenses/modernc-sqlite-LICENSE | modernc.org/sqlite@v1.60.1/LICENSE | identical |
| licenses/modernc-sqlite-LICENSE-3RD-PARTY | modernc.org/sqlite@v1.60.1/LICENSE-3RD-PARTY.md | identical |
| licenses/modernc-sqlite-LICENSE-SQLITE | modernc.org/sqlite@v1.60.1/LICENSE-SQLITE | identical |
| licenses/modernc-sqlite-LICENSE-SQLITE_VEC | modernc.org/sqlite@v1.60.1/LICENSE-SQLITE_VEC | identical |
| licenses/x-sys-LICENSE | golang.org/x/sys@v0.48.0/LICENSE | identical |
| licenses/x-sys-PATENTS | golang.org/x/sys@v0.48.0/PATENTS | identical |

`TOTAL=20 FAIL=0`. No truncated, altered, or wrong-licence text was found; the MPL-2.0 text
(`go-sql-driver-mysql-LICENSE`, 16726 B, sha256 `fab3dd6b…`) begins "Mozilla Public License
Version 2.0" and is byte-identical to the v1.10.1 module text. The texts also match
`docs/licenses/*` at commit `1f8e013` (`git show 1f8e013:docs/licenses/<name> | cmp -` → 20/20, 0 mismatches),
so the package did not diverge from the repository it claims to come from.

`licenses/modernc-memory-LICENSE-LOGO` is 62 B and contains only a URL
(`https://commons.wikimedia.org/wiki/File:Memory_infra_logo.png`); that is the upstream
`LICENSE-LOGO` content verbatim, not a truncated licence. No attribution obligation is created
by it (no code, no asset is linked; verified in §5).

## 5. NOTICE grouping honesty

**Linked set check — FAIL (P2-1).** `go version -m` and the import graph list 10 third-party
modules, but NOTICE's "Linked into the binary" table names 12:

```console
$ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go list -deps -f '{{.ImportPath}}' ./cmd/maskriver | wc -l
204
$ go version -m maskriver | grep -c '^	dep'
10        # edwards25519, go-humanize, mysql, uuid, bigfft, x/sys, libc, mathutil, memory, sqlite
$ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go list -deps ./cmd/maskriver | grep -cE 'mattn/go-isatty|ncruces/go-strftime'
0
$ strings -a maskriver | grep -c 'mattn/go-isatty'; echo $?
0         # and 0 for ncruces/go-strftime
```

`github.com/mattn/go-isatty` and `github.com/ncruces/go-strftime` are **not linked for
linux/amd64**, yet both are listed under "Linked into the binary". Reason from the module
sources: `modernc.org/libc@v1.77.1/libc.go` carries `//go:build !linux || mips64le` and is the
only importer of isatty; `libc_unix.go` (`//go:build unix && !(linux && (amd64 || …))`) and
`libc_windows.go` are the only importers of go-strftime. Both files are excluded on this
target. Nothing is missing from the linked group — the error is over-inclusion, which is the
harmless direction for attribution (two extra MIT texts shipped), but the statement is false
as written.

**Musl material genuinely compiled — PASS.** For `GOOS=linux GOARCH=amd64`,
`go list -f '{{.GoFiles}}' modernc.org/libc` selects `libc_musl.go`, `libc_musl_linux_amd64.go`,
`etc_musl.go`, `mem_musl.go`, `native_musl.go`, `pthread_musl.go`, `syscall_musl.go`,
`syscall_musl_time64.go`, while `libc_linux_amd64.go` is in `IgnoredGoFiles` because of its
`//go:build !(linux && amd64)` tag, and `atomic_nonmusl.go` is likewise ignored.
`modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md` contains the musl section ("musl libc",
3 matches). NOTICE's claim "selects its musl-translated implementation on linux/amd64 without
a build tag" is accurate.

**sqlite-vec not linked — PASS.** `modernc.org/sqlite/vec` is a separate generated package
(`vec/vec_linux_amd64.go`, `//go:build linux && amd64`, `package vec`) that only takes effect
through an explicit blank import (documented in `vec/patches.go`: `_ "modernc.org/sqlite/vec"`).
No non-test file in the module imports it, `modernc.org/sqlite/vec` is absent from the 204-package
dependency list, and the binary contains 0 occurrences of `sqlite_vec`, `sqlite3_vec`, `vec_init`,
`vec0`. Correctly classified as "no linked code".

**Logo not linked — PASS.** No `.go` file in `modernc.org/memory@v1.12.1` references
`LICENSE-LOGO` or the logo URL.

## 6. MPL-2.0-SOURCE.md

- Version named: **v1.10.1**. Version actually linked (`go version -m`):
  `dep github.com/go-sql-driver/mysql v1.10.1 h1:arlSnNLq6a5yxGxV7qg9lF4j0C+KwD6NbQyKr9QL6ME=`.
  `go.mod` at `1f8e013` also pins `v1.10.1`. Match.
- Stated source path works, exactly as written:

```console
$ cd /tmp/mpldl && go mod init mplcheck && go mod download github.com/go-sql-driver/mysql@v1.10.1
EXIT=0
$ go mod download -x github.com/go-sql-driver/mysql@v1.10.1   # EXIT=0
$ go mod download -json … → "Dir": "<GOMODCACHE>/github.com/go-sql-driver/mysql@v1.10.1"
```

- The module digest in the binary equals the proxy digest for that version, and the shipped
  licence is byte-identical to that module's LICENSE (§4). PASS.
  Not verified: reachability of the GitHub tag over the network (no network access used);
  the module proxy resolution already establishes the version exists upstream.

## 7. Binary invocation

| Command | Exit | Output |
|---|---|---|
| `./maskriver --version` | 0 | `MaskRiver 0.1.0-dev` |
| `./maskriver --help` | 0 | usage block, 7 lines |
| `./maskriver` (argument-free) | **0** | usage block; exits 0, not a refusal |
| `./maskriver scan` (subcommand, no flags) | 2 | `operation refused or incomplete (sanitized; no database values or DSN)` |

The release record's claim about `./maskriver scan` → 2 is accurate. The bare invocation exits 0
by design (`main.go`: help path); noted as P2-5, not a defect against any stated invariant.

## 8. Synthetic SQLite smoke test (package binary only)

Fixture created independently with Python `sqlite3`:
`people(id INTEGER PRIMARY KEY, email TEXT NOT NULL, full_name TEXT)`, 3 rows
`user{1,2,3}@example.invalid`. Source and target are two independent 8192-byte files,
identical before the run (sha256 `6c87dfce9f1a596b03e3fe5e54126a8f9e39d1f90ae1b694a23afa7f7837af62`).
Reviewer key: fresh Ed25519 key from `openssl genpkey` (not the test fixture seed), 64-byte raw
private key file; trust file `{"version":"maskriver-trust-v1","keys":[{"signer_key_id":"reviewer-1",…}]}`.

| Step (each its own process) | Command core | Exit | Output |
|---|---|---|---|
| scan | `scan --source … --target … --table people --column email --min-samples 3` | **0** | `scan: complete=true columns=1 issues=0` |
| dry-run preview | `mask … --strategy fake_email --key-id synthetic-key --key-file …` | **0** | `preview: dry-run=true rows=3 complete=true (no row values shown)` |
| preview was read-only | `cmp target.db source.db` | 0 | identical (same sha256 as before preview) |
| approve (separate process) | `approve … --history-dir … --trust-file … --reviewer-key-id reviewer-1 --reviewer-key-file …` | **0** | `approve: stored=true scopes=1 valid_for=24h0m0s` |
| apply (new process) | `mask … --history-dir … --trust-file … --apply --approved` | **0** | `apply: complete=true committed_batches=1 committed_rows=3 outcome_unknown=false` |
| strict validate | `validate … --approved --strict` | **0** | `validate: pass=true tables=1 rows=3 issues=0` |
| source untouched | re-read `source.db` | — | still `user1..3@example.invalid` |
| target masked | read `target.db` | — | 3 rows, 3 distinct, emails now `<32 hex>@maskriver.local`, `full_name` untouched |

Refusals (all from the packaged binary, synthetic data):

| Negative case | Exit | Observed |
|---|---|---|
| `mask … --apply --approved` with trust/history but **no stored decision** | **1** | `apply: complete=false committed_batches=0 committed_rows=0`; target still byte-identical to source |
| `mask … --apply --approved` without `--trust-file/--history-dir` | **2** | `operation refused or incomplete (sanitized…)` |
| `mask … --apply` without `--approved` | **2** | `operation refused or incomplete (sanitized…)` |
| UNIQUE-constrained masked column — preview | **1** | `operation refused or incomplete (sanitized…)` |
| UNIQUE-constrained masked column — `--apply --approved` with a valid stored decision | **1** | `apply: complete=false …`; target byte-identical to source (no write) |

The refusal sanitisation held: no fixture value, key bytes, or absolute path appeared in any
refusal output. This is not a happy-path-only smoke.

## 9. Leak scan

Archive text files (`grep -rnaE`, binary excluded): `/home/`, `/Users/`, `C:\Users`,
`PRIVATE KEY`, `BEGIN OPENSSH`, `DSN`, `password=`, `X-Amz-Signature`, `sig=`, `token=`,
`example.invalid@`, `.git` → **NONE**. The only `@`-bearing strings are upstream copyright
e-mail addresses inside licence texts (`mattn.jp@gmail.com`, `edsrzf@gmail.com`), which are the
required, unmodified upstream notices.

Binary (`strings -a maskriver | grep -c`): `/home/` 0, `/Users/` 0, `C:\Users` 0,
`PRIVATE KEY` 0, `X-Amz-Signature` 0, `AWSAccessKey` 0, `.git/` 0. Only 4 `https://` strings,
all stdlib/driver messages (`go.dev/pkg/crypto/rsa`, `go.dev/issue/66821`,
`github.com/go-sql-driver/mysql/wiki/...`). Absolute-path constants are stdlib-only
(`/proc/self/…`, `/etc/ssl/certs/…`, `/dev/tty`).

Archive contents: no `.git`, no `.db`/`-wal`/`-shm`, no `.decision`, no key files, no test
fixtures, no build cache. Package is 27 files, all accounted for in §3. No leak found.

## 10. Binary construction

```console
$ file maskriver
ELF 64-bit LSB executable, x86-64, … statically linked, … stripped
$ readelf -d maskriver
There is no dynamic section in this file.
$ go version -m maskriver
go1.26.3
path  github.com/haoyuehx/maskriver/cmd/maskriver
mod   github.com/haoyuehx/maskriver v0.0.0-20261019024838-1f8e0130f71a
dep   … 10 third-party modules, exactly the linux/amd64 linked set of §5 …
build -buildmode=exe -compiler=gc -trimpath=true CGO_ENABLED=0 GOARCH=amd64 GOOS=linux GOAMD64=v1
build vcs=git vcs.revision=1f8e0130f71a3273a19f10d23d1189241c902fff vcs.time=2026-10-19T02:48:38Z vcs.modified=false
```

- Stripped, static, no interpreter, `-trimpath` and `vcs.modified=false` confirmed. `go.mod`
  declares 12 modules; the binary records 10, matching the import graph (see P2-1).
- No module outside `go.mod` appears in the build info. No test-only or unexpected module.
- Build-host paths: `maskriver-m1-linux-amd64` 0, `/tmp/rel` 0, `haoyue/Project` 0, `.gvm` 0,
  `GOMODCACHE` 0 occurrences.
- `--version` matches `cmd/maskriver/main.go` at `1f8e013` (`const version = "0.1.0-dev"`) and
  `BUILD-INFO.md` (`Version 0.1.0-dev`, commit `1f8e013…`, Go `go1.26.3`).

## 11. Independent rebuild (reproducibility)

Fresh clone of the repository into `/tmp/rev-fresh-50311/repro`, `git checkout 1f8e013…`,
then the committed builder and the documented tar command:

```console
$ ./scripts/build-release.sh /tmp/rev-fresh-50311/rebuilt/maskriver-m1-linux-amd64-1f8e013
$ diff -r rebuilt/maskriver-m1-linux-amd64-1f8e013 shipped/maskriver-m1-linux-amd64-1f8e013
EXIT=0                                    # every file byte-identical, binary included
$ sha256sum */maskriver
416eaf145ec55d3146c13bcece8de07906eb745fd20b4f8d8dc232872ec91859  (both)
$ tar --sort=name --owner=0 --group=0 --numeric-owner --mtime='2026-10-19 00:00Z' -czf repro2.tar.gz …
$ sha256sum repro2.tar.gz
1b7a04209e7f3ea2da998c067f186817d0f7b44147a19eeb258f662a248319f6   # ≠ shipped
```

The only difference is the tar mode bits of the 20 licence files: shipped `-r--r--r--` (0444),
fresh-clone rebuild `-rw-rw-r--` (0664), because `docs/licenses/*` are 0444 in the build
worktree but git records only the executable bit, so a clone checks them out umask-derived.
After `chmod 444 rebuilt/.../licenses/*` and re-tarring, the archive digest becomes exactly
`e50ec59744547888d6143f71538319020807dddad7431e70727df9d708ef8b6f`, i.e. the shipped artifact
is reproducible bit-for-bit from commit `1f8e013` with the documented procedure (P2-3).

## 12. Findings

### P0 — none

### P1 — none

No missing or altered licence text, no NOTICE reference to a non-existent file, no leak, no
integrity failure, no unlisted extra file.

### P2 (non-blocking; correct before the next package)

- **P2-1 — `NOTICE.md`, "Linked into the binary" table: two false linkage claims.**
  `github.com/mattn/go-isatty` and `github.com/ncruces/go-strftime` are listed as compiled into
  the linux/amd64 binary; they are not (absent from `go list -deps` for the target, absent from
  `go version -m`, 0 string hits; excluded by `libc.go`'s `!linux || mips64le` and
  `libc_unix.go`'s `!(linux && amd64…)` tags). Attribution-wise this errs safe (two extra MIT
  texts), but the sentence "ships the complete, unmodified licence text of every third-party
  component that is compiled into the binary" is followed by a table that overstates the linked
  set. Root cause: `scripts/build-release.sh` hardcodes these rows while its own comment claims
  the table is generated from `docs/attribution.md` (which carries the same two rows, lines 29–30).
  Fix: move the two rows to a "module graph only / not linked on this platform" group, or derive
  the table from `go list -deps`/build info.
- **P2-2 — `docs/releases/m1-candidate-linux-amd64-1f8e013.md` §2 counting error.**
  It records `sha256sum -c SHA256SUMS` as "24/24 OK"; the file has **25** entries and all
  **25** verify (4 documents + 20 licence texts + binary). The result is better than recorded,
  but the number is wrong.
- **P2-3 — archive digest is not reproducible from a fresh clone without a mode fix.**
  Payload bytes are identical; only the 0444 vs 0664 mode of `docs/licenses/*` differs, which
  changes the tarball digest (`1b7a0420…` vs `e50ec597…`). Fix: `chmod 0444` the licence files
  in `build-release.sh` after `cp`, and state the expected mode in the record.
- **P2-4 — `SHA256SUMS` does not cover `MANIFEST`.** By construction neither `MANIFEST` nor
  `SHA256SUMS` is self-listed, so MANIFEST's integrity is not cryptographically pinned (its
  content is covered indirectly: every file it names is hashed). Informational.
- **P2-5 — inconsistent no-argument semantics.** `./maskriver` prints usage and exits 0, while
  `./maskriver scan` (no flags) exits 2. Harmless, but "no arguments" is not a refusal.
- **P2-6 — build date/time metadata.** All archive entries carry 2026-10-19 08:00 (+08:00),
  matching `--mtime='2026-10-19 00:00Z'`; no host or author metadata is embedded (0/0 owner).

### Not verified

- GitHub tag `v1.10.1` reachability over the network (offline review); the module proxy
  resolution to the same version and digest was verified instead.
- macOS/Windows binaries: not shipped, not evaluated.
- Detection quality, MySQL CLI, checkpoint/resume, exactly-once: outside this artifact's claims
  and explicitly disclaimed in `BUILD-INFO.md`; not exercised.

## 13. Verdict

Every integrity, licence-completeness, leak, construction, reproducibility and smoke check
passed against the actual bytes; the only failures are documentation-accuracy items (P2-1,
P2-2) that do not break licence compliance, and they err toward over-attribution. The package's
own stated boundaries (synthetic/authorised disposable SQLite, linux/amd64, no anonymity claim)
are consistent with what the binary does.

Candidate verdict: RELEASE CANDIDATE READY
