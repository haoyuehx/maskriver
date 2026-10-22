#!/usr/bin/env bash
# Build the MaskRiver Linux amd64 release package and deterministic archive.
#
# Usage: scripts/build-release.sh <output-root>
#
# Creates under <output-root>:
#   maskriver-<version>-linux-amd64/            package directory
#   maskriver-<version>-linux-amd64.tar.gz      deterministic archive
#   SHA256SUMS                                  archive digest
#
# The build requires a clean Git worktree. It refuses dangerous output paths and
# any non-empty existing output root. The archive timestamp is the commit time
# (SOURCE_DATE_EPOCH), file modes are pinned, and tar/gzip metadata is fixed so
# the same commit and toolchain produce the same bytes.
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
OUT_ARG=${1:?usage: build-release.sh <output-root>}

export LC_ALL=C
export TZ=UTC
export GOFLAGS=-trimpath
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOAMD64=v1

cd "$REPO"

# --- version and source identity ------------------------------------------------
VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' cmd/maskriver/main.go)
if [[ -z "$VERSION" || ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "build-release: invalid or missing CLI version in cmd/maskriver/main.go" >&2
  exit 1
fi
SHA=$(git rev-parse HEAD)
SHORT=$(git rev-parse --short=7 HEAD)
if [[ -n "$(git status --porcelain)" ]]; then
  echo "build-release: refusing to build from a dirty worktree; commit or stash first" >&2
  exit 1
fi
if [[ -n "${RELEASE_TAG:-}" && "$RELEASE_TAG" != "v$VERSION" ]]; then
  echo "build-release: RELEASE_TAG=$RELEASE_TAG does not match version v$VERSION" >&2
  exit 1
fi
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
BUILD_TIME=$(date -u -d "@$SOURCE_DATE_EPOCH" +%Y-%m-%dT%H:%M:%SZ)
GOVERSION=$(go env GOVERSION)
PKG_NAME="maskriver-v$VERSION-linux-amd64"
if git tag --points-at HEAD | grep -qx "v$VERSION"; then
  RELEASE_TAG_STATE="v$VERSION (tag present at HEAD)"
else
  RELEASE_TAG_STATE="v$VERSION (candidate: tag not yet created)"
fi

# --- safe output path -----------------------------------------------------------
# Resolve without requiring the path to exist yet.
OUT=$(realpath -m -- "$OUT_ARG")
case "$OUT" in
  /|"$HOME"|"$REPO"|"$REPO"/*)
    echo "build-release: refusing dangerous output root: $OUT" >&2
    exit 1
    ;;
esac
if [[ ! -d "$(dirname "$OUT")" ]]; then
  echo "build-release: parent directory does not exist: $(dirname "$OUT")" >&2
  exit 1
fi
if [[ -e "$OUT" ]]; then
  if [[ ! -d "$OUT" ]]; then
    echo "build-release: output root exists and is not a directory: $OUT" >&2
    exit 1
  fi
  if [[ -n "$(find "$OUT" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    echo "build-release: output root is not empty; refusing to delete or reuse it: $OUT" >&2
    exit 1
  fi
else
  mkdir -p "$OUT"
fi

PKG="$OUT/$PKG_NAME"
ARCHIVE="$OUT/$PKG_NAME.tar.gz"
mkdir -p "$PKG/licenses"

# --- build binary ---------------------------------------------------------------
echo "building $VERSION ($SHORT) for linux/amd64 with $GOVERSION"
go build -mod=readonly -buildvcs=true -ldflags "-s -w -buildid=" \
  -o "$PKG/maskriver" ./cmd/maskriver

# Record and verify the embedded provenance immediately.
go version -m "$PKG/maskriver" >"$OUT/.go-version-m.txt"
if ! grep -q "vcs.revision=$SHA$" "$OUT/.go-version-m.txt"; then
  echo "build-release: embedded vcs.revision does not match HEAD" >&2
  exit 1
fi
if ! grep -q "vcs.modified=false$" "$OUT/.go-version-m.txt"; then
  echo "build-release: embedded vcs.modified is not false" >&2
  exit 1
fi
if ! grep -q "GOOS=linux$" "$OUT/.go-version-m.txt" ||
   ! grep -q "GOARCH=amd64$" "$OUT/.go-version-m.txt"; then
  echo "build-release: embedded platform is not linux/amd64" >&2
  exit 1
fi
rm -f "$OUT/.go-version-m.txt"

# --- copy project texts and licences -------------------------------------------
cp LICENSE "$PKG/LICENSE"
cp docs/licenses/* "$PKG/licenses/"
cp "$(go env GOROOT)/LICENSE" "$PKG/licenses/go-toolchain-LICENSE"
cp "$(go env GOROOT)/PATENTS" "$PKG/licenses/go-toolchain-PATENTS"
cp docs/releases/v"$VERSION".md "$PKG/RELEASE-NOTES.md" 2>/dev/null || {
  echo "build-release: missing docs/releases/v$VERSION.md" >&2
  exit 1
}

# --- link set (module paths only, sorted, unique) ------------------------------
LINKED_MODULES="$OUT/.linked-modules"
if ! go list -deps -f '{{with .Module}}{{.Path}}{{end}}' ./cmd/maskriver >"$OUT/.deps.raw"; then
  echo "build-release: go list -deps failed" >&2
  exit 1
fi
grep -v '^$' "$OUT/.deps.raw" | grep -v '^github.com/haoyuehx/maskriver$' | LC_ALL=C sort -u >"$LINKED_MODULES"
if [[ ! -s "$LINKED_MODULES" ]]; then
  echo "build-release: empty dependency link set" >&2
  exit 1
fi
rm -f "$OUT/.deps.raw"

license_for() {
  case "$1" in
    modernc.org/sqlite) echo "modernc-sqlite-LICENSE" ;;
    modernc.org/libc) echo "modernc-libc-LICENSE" ;;
    modernc.org/mathutil) echo "modernc-mathutil-LICENSE" ;;
    modernc.org/memory) echo "modernc-memory-LICENSE" ;;
    github.com/go-sql-driver/mysql) echo "go-sql-driver-mysql-LICENSE" ;;
    filippo.io/edwards25519) echo "edwards25519-LICENSE" ;;
    github.com/dustin/go-humanize) echo "go-humanize-LICENSE" ;;
    github.com/google/uuid) echo "google-uuid-LICENSE" ;;
    github.com/remyoudompheng/bigfft) echo "bigfft-LICENSE" ;;
    golang.org/x/sys) echo "x-sys-LICENSE" ;;
    *) return 1 ;;
  esac
}

license_label_for() {
  case "$1" in
    github.com/go-sql-driver/mysql) echo "MPL-2.0" ;;
    github.com/dustin/go-humanize|github.com/mattn/go-isatty|github.com/ncruces/go-strftime) echo "MIT" ;;
    *) echo "BSD-3-Clause" ;;
  esac
}


# --- NOTICE ---------------------------------------------------------------------
NOTICE="$PKG/NOTICE.md"
{
  echo "# NOTICE"
  echo
  echo "MaskRiver is MIT licensed; see LICENSE."
  echo
  echo "This package ships the complete, unmodified licence text of every third-party"
  echo "module compiled into the \`maskriver\` binary for linux/amd64, plus additional"
  echo "third-party material shipped inside those modules. The link set is derived"
  echo "from \`go list -deps\` for this exact target, not from a static list."
  echo
  echo "## Modules linked into the binary"
  echo
  echo "| Module | Licence | Text in this package |"
  echo "|---|---|---|"
  while IFS= read -r module; do
    text=$(license_for "$module") || {
      echo "build-release: no licence mapping for linked module: $module" >&2
      exit 1
    }
    [[ -f "$PKG/licenses/$text" ]] || {
      echo "build-release: missing licence text: licenses/$text" >&2
      exit 1
    }
    licence=$(license_label_for "$module")
    echo "| $module | $licence | licenses/$text |"
  done <"$LINKED_MODULES"
  echo
  echo "## Additional texts shipped with the binary"
  echo
  echo "| Module | Material | Text in this package |"
  echo "|---|---|---|"
  while IFS='|' read -r module material text; do
    [[ -f "$PKG/licenses/$text" ]] || {
      echo "build-release: NOTICE references a missing text: licenses/$text" >&2
      exit 1
    }
    echo "| $module | $material | licenses/$text |"
  done <<'EXTRA'
Go toolchain/runtime|Go standard library and runtime object code|go-toolchain-LICENSE
Go toolchain/runtime|Additional IP Rights Grant (patents)|go-toolchain-PATENTS
modernc.org/libc|Go / musl / go-netdb / nixpkgs|modernc-libc-LICENSE-3RD-PARTY
modernc.org/sqlite|bundled third-party notices|modernc-sqlite-LICENSE-3RD-PARTY
modernc.org/sqlite|SQLite itself (public domain)|modernc-sqlite-LICENSE-SQLITE
modernc.org/memory|Go runtime material|modernc-memory-LICENSE-GO
modernc.org/memory|mmap-go material|modernc-memory-LICENSE-MMAP-GO
golang.org/x/sys|patent grant|x-sys-PATENTS
EXTRA
  echo
  echo "## Texts shipped upstream but not linked by this build"
  echo
  echo "These files are included because the upstream modules ship them. They are"
  echo "not part of this binary's link set and are not required by it."
  echo
  echo "| Component | Material | Text in this package |"
  echo "|---|---|---|"
  while IFS='|' read -r component material text; do
    [[ -f "$PKG/licenses/$text" ]] || {
      echo "build-release: NOTICE references a missing text: licenses/$text" >&2
      exit 1
    }
    echo "| $component | $material | licenses/$text |"
  done <<'UNLINKED'
modernc.org/sqlite|sqlite-vec (upstream text)|modernc-sqlite-LICENSE-SQLITE_VEC
modernc.org/memory|project logo reference (URL)|modernc-memory-LICENSE-LOGO
github.com/mattn/go-isatty|module requirement, not linked by this build|go-isatty-LICENSE
github.com/ncruces/go-strftime|module requirement, not linked by this build|go-strftime-LICENSE
UNLINKED
  echo
  echo "## MPL-2.0"
  echo
  echo "\`github.com/go-sql-driver/mysql\` is covered by the Mozilla Public License 2.0."
  echo "Its complete text is licenses/go-sql-driver-mysql-LICENSE. How to obtain the"
  echo "corresponding source form of the MPL-covered files is described in"
  echo "MPL-2.0-SOURCE.md."
} >"$NOTICE"

# --- MPL-2.0 source offer -------------------------------------------------------
MYSQL_VERSION=$(go list -m -f '{{.Version}}' github.com/go-sql-driver/mysql)
[[ -n "$MYSQL_VERSION" ]] || { echo "build-release: cannot resolve mysql module version" >&2; exit 1; }
cat >"$PKG/MPL-2.0-SOURCE.md" <<MPL
# Obtaining the source of the MPL-2.0 covered files

\`maskriver\` links \`github.com/go-sql-driver/mysql\` at the version pinned in this
release's \`go.mod\`: **$MYSQL_VERSION**, which is covered by the Mozilla Public
License 2.0.

The complete licence text is \`licenses/go-sql-driver-mysql-LICENSE\`.

To obtain the corresponding source form of the MPL-covered files:

\`\`\`sh
go mod download -x github.com/go-sql-driver/mysql@$MYSQL_VERSION
# the module source is then at \$(go env GOMODCACHE)/github.com/go-sql-driver/mysql@$MYSQL_VERSION
\`\`\`

The same version is obtainable from the upstream repository
\`https://github.com/go-sql-driver/mysql\` at tag \`$MYSQL_VERSION\`.

MaskRiver does not modify those files: they are used as published by the module,
unmodified, through the Go module mechanism.
MPL

# --- SBOM (actual link set) ----------------------------------------------------
{
  echo "{"
  echo "  \"format\": \"maskriver-sbom-v1\","
  echo "  \"project\": \"github.com/haoyuehx/maskriver\","
  echo "  \"version\": \"$VERSION\","
  echo "  \"commit\": \"$SHA\","
  echo "  \"platform\": \"linux/amd64\","
  echo "  \"go\": \"$GOVERSION\","
  echo "  \"modules\": ["
  first=1
  while IFS= read -r module; do
    version=$(go list -m -f '{{.Version}}' "$module")
    sum=$(go list -m -f '{{.Sum}}' "$module")
    [[ -n "$version" ]] || { echo "build-release: cannot resolve version for $module" >&2; exit 1; }
    if [[ $first -eq 0 ]]; then echo ","; fi
    first=0
    printf '    {"path": "%s", "version": "%s", "sum": "%s"}' "$module" "$version" "$sum"
  done <"$LINKED_MODULES"
  echo
  echo "  ]"
  echo "}"
} >"$PKG/SBOM.json"

# --- build info ----------------------------------------------------------------
cat >"$PKG/BUILD-INFO.md" <<INFO
# Build information

| Field | Value |
|---|---|
| Project | MaskRiver |
| Version | $VERSION |
| Release tag | $RELEASE_TAG_STATE |
| Commit | $SHA |
| Commit short | $SHORT |
| Platform | linux/amd64 |
| Go | $GOVERSION |
| Build mode | CGO_ENABLED=0, -trimpath, -buildvcs=true, GOAMD64=v1, -ldflags "-s -w -buildid=" |
| Source date epoch | $SOURCE_DATE_EPOCH ($BUILD_TIME) |
| Frozen contracts | APIRevision=m1a-v1, StreamingProtocolRevision=m2-stream-v1, ApprovalBindingRevision=m2-approval-binding-v1, RecoveryProtocolRevision=m2-recovery-v1 |
| Worktree | clean (vcs.modified=false) |

## Scope of use

Authorised, isolated, disposable **SQLite or MySQL 8.0.46** copies only.
Streaming, outbox and recovery are SQLite-only. Masking is not anonymisation.
Deterministic output is not collision-free. There is no generic distributed
exactly-once guarantee and no proof of real power-loss durability.

## Verified in this candidate's test suite

- SQLite multi-table \`scan\` → \`preview\` → Ed25519 \`approve\` (separate process)
  → \`mask --apply\` in a new process → strict \`validate\`.
- Explicit M2 \`--streaming --apply\` with a cryptographic approval, transactional
  outbox receipts and an explicit resource budget.
- Local SQLite \`m2-recovery-v1\`: explicit \`init-recovery\`, read-only \`status\`,
  explicit \`--resume\`, atomic checkpoint/receipt/business transaction, bounded
  prefix/suffix verification, commit-unknown reconciliation and idempotent replay.
- Real CLI child-process interruption at batches 1/10/100/500, between tables,
  after validation and before/after commit; resumed output equals the
  uninterrupted baseline.
- MySQL 8.0.46 CLI in the isolated CI container: Scan -> Preview -> separate
  Approve -> Apply -> strict Validate on two distinct databases, with
  same-database and streaming refusal.

## Not verified — do not claim

MySQL recovery/streaming · Windows/macOS streaming or recovery · WAL recovery ·
generic distributed exactly-once · cross-store mapping atomicity · real power-loss
durability · production-grade PII identification accuracy · anonymisation ·
collision-free replacements · tamper-proof history · MySQL versions other than
8.0.46 · bounded RSS under all unbounded conditions · every filesystem failure
mode.
INFO

# --- package README / quickstart ----------------------------------------------
cat >"$PKG/README.md" <<'README'
# MaskRiver — data masking CLI

This is a **Linux amd64 release package**. Use it only on authorised, isolated,
disposable SQLite or MySQL 8.0.46 copies. See `RELEASE-NOTES.md` for the exact
supported scope and non-claims.

```sh
sha256sum -c SHA256SUMS
./maskriver --version
./maskriver --help
```

M1 workflow (two separate existing SQLite files):

```sh
maskriver scan    --source /path/source.db --target /path/target.db --scope table.column ...
maskriver mask    --source /path/source.db --target /path/target.db --scope table.column \
                  --strategy fake_email --key-id demo --key-file key.bin       # preview
maskriver approve --source /path/source.db --target /path/target.db --scope table.column \
                  --strategy fake_email --key-id demo \
                  --trust-file trust.json --history-dir history \
                  --reviewer-key-id reviewer-1 --reviewer-key-file reviewer.key --reviewer-id reviewer
maskriver mask    --source /path/source.db --target /path/target.db --scope table.column \
                  --strategy fake_email --key-id demo --key-file key.bin \
                  --trust-file trust.json --history-dir history --approved --apply
maskriver validate --source /path/source.db --target /path/target.db --scope table.column \
                  --strategy fake_email --key-id demo --key-file key.bin --strict
```

MySQL 8.0.46 M1 (two DSNs for distinct databases):

```sh
maskriver scan --dialect mysql --source '<dsn>' --target '<dsn>' --scope people.born ...
# mask/approve/apply/validate use the same --dialect mysql and DSN flags.
```

M2 streaming with budget, outbox and recovery:

```sh
maskriver init-outbox  --target /path/target.db
maskriver init-recovery --target /path/target.db
maskriver approve --streaming ... --trust-file ... --history-dir ... --reviewer-key-id ... \
                  --reviewer-key-file ... --reviewer-id ...
maskriver mask --streaming --apply --approved --budget-file budget.json \
               --receipt-page-size 8 --run-id <32-hex> ... --key-file ...
maskriver status --target /path/target.db --run-id <32-hex>
maskriver mask --streaming --apply --approved --resume --run-id <32-hex> ... --key-file ...
```

No flag or environment variable enables **business data** writes without
`--apply`; streaming also requires a valid Ed25519 approval bound to the plan.
`init-outbox`, `init-recovery` and `approve`/`revoke` are explicit schema or
history writes and do not need `--apply`. See `RELEASE-NOTES.md` and
`BUILD-INFO.md` for the exact supported scope and non-claims.
README

# --- modes, manifest and checksums --------------------------------------------
find "$PKG" -type d -exec chmod 0755 {} +
find "$PKG" -type f -exec chmod 0644 {} +
chmod 0755 "$PKG/maskriver"

(
  cd "$PKG"
  find . -type f ! -name MANIFEST ! -name SHA256SUMS | sed 's|^\./||' | LC_ALL=C sort >MANIFEST
  while IFS= read -r file; do
    sha256sum "$file"
  done <MANIFEST >SHA256SUMS
  chmod 0644 MANIFEST SHA256SUMS
)

rm -f "$LINKED_MODULES"

# --- deterministic archive -----------------------------------------------------
# gzip -n omits name/time; tar pins order, ownership and mtime. Source date is
# the commit time so a rebuild of the same commit is byte-identical.
(
  cd "$OUT"
  tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner \
      --format=gnu -cf - "$PKG_NAME" | gzip -n -9 >"$ARCHIVE"
  sha256sum "$(basename "$ARCHIVE")" >SHA256SUMS
  chmod 0644 SHA256SUMS
)

echo "package: $PKG"
echo "archive: $ARCHIVE"
echo "archive sha256: $(cut -d' ' -f1 "$OUT/SHA256SUMS")"
