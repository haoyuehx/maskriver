#!/bin/sh
# Build the Linux amd64 M1 release candidate package.
#
# Produces, under $OUT:
#   maskriver                        the executable
#   LICENSE                          project licence (MIT)
#   NOTICE.md                        attribution index over every applicable text
#   licenses/                        complete, unmodified third-party texts
#   MPL-2.0-SOURCE.md                how to obtain the source of the MPL-2.0 files
#   BUILD-INFO.md                    version, commit, platform, scope of use
#   MANIFEST                         every file, in package order
#   SHA256SUMS                       digest of every file except itself
#
# The package contains no key material, no decision records and no databases.
set -eu

REPO=$(cd "$(dirname "$0")/.." && pwd)
OUT=${1:?usage: build-release.sh <output-dir>}
SHA=$(cd "$REPO" && git rev-parse HEAD)
SHORT=$(echo "$SHA" | cut -c1-7)
VERSION=$(cd "$REPO" && sed -n 's/^const version = "\(.*\)"/\1/p' cmd/maskriver/main.go)

rm -rf "$OUT"
mkdir -p "$OUT/licenses"

echo "building $VERSION ($SHORT) for linux/amd64"
cd "$REPO"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=-trimpath \
	go build -buildvcs=true -ldflags "-s -w" -o "$OUT/maskriver" ./cmd/maskriver

cp LICENSE "$OUT/LICENSE"
cp docs/licenses/* "$OUT/licenses/"

# NOTICE.md is generated from docs/attribution.md's tables so the index cannot
# drift from the files that are actually shipped.
{
	echo "# NOTICE"
	echo
	echo "MaskRiver is MIT licensed; see LICENSE."
	echo
	echo "This package ships the complete, unmodified licence text of every third-party"
	echo "component that is compiled into the \`maskriver\` binary for linux/amd64, plus"
	echo "the texts that dependencies ship inside themselves, plus a small number of"
	echo "upstream texts that carry no linked code. The last group is marked as such and"
	echo "is included because the upstream project ships it, not because it is linked."
	echo
	echo "## Linked into the binary"
	echo
	echo "Derived from \`go list -deps\` for the target platform, not from a static list, so"
	echo "the index can only claim what the build actually links."
	echo
	echo "| Component | Licence | Text in this package |"
	echo "|---|---|---|"
	cd "$REPO"
	go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./cmd/maskriver \
		| grep -v '^github.com/haoyuehx/maskriver' | sort >"$OUT/.linked"
	cd "$OUT"
	while read -r mod; do
		case "$mod" in
		github.com/haoyuehx/maskriver*) continue ;;
		modernc.org/sqlite*) echo "| modernc.org/sqlite | BSD-3-Clause | licenses/modernc-sqlite-LICENSE |" ;;
		modernc.org/libc*) echo "| modernc.org/libc | BSD-3-Clause | licenses/modernc-libc-LICENSE |" ;;
		modernc.org/mathutil*) echo "| modernc.org/mathutil | BSD-3-Clause | licenses/modernc-mathutil-LICENSE |" ;;
		modernc.org/memory*) echo "| modernc.org/memory | BSD-3-Clause | licenses/modernc-memory-LICENSE |" ;;
		github.com/go-sql-driver/mysql/*|github.com/go-sql-driver/mysql) echo "| github.com/go-sql-driver/mysql | MPL-2.0 | licenses/go-sql-driver-mysql-LICENSE |" ;;
		filippo.io/edwards25519*) echo "| filippo.io/edwards25519 | BSD-3-Clause | licenses/edwards25519-LICENSE |" ;;
		github.com/dustin/go-humanize/*|github.com/dustin/go-humanize) echo "| github.com/dustin/go-humanize | MIT | licenses/go-humanize-LICENSE |" ;;
		github.com/google/uuid/*|github.com/google/uuid) echo "| github.com/google/uuid | BSD-3-Clause | licenses/google-uuid-LICENSE |" ;;
		github.com/mattn/go-isatty/*|github.com/mattn/go-isatty) echo "| github.com/mattn/go-isatty | MIT | licenses/go-isatty-LICENSE |" ;;
		github.com/ncruces/go-strftime/*|github.com/ncruces/go-strftime) echo "| github.com/ncruces/go-strftime | MIT | licenses/go-strftime-LICENSE |" ;;
		github.com/remyoudompheng/bigfft/*|github.com/remyoudompheng/bigfft) echo "| github.com/remyoudompheng/bigfft | BSD-3-Clause | licenses/bigfft-LICENSE |" ;;
		golang.org/x/sys/*|golang.org/x/sys) echo "| golang.org/x/sys | BSD-3-Clause | licenses/x-sys-LICENSE |" ;;
		*) echo "| UNMAPPED: $mod | — | missing from NOTICE |" ;;
		esac
	done <"$OUT/.linked" | awk '!seen[$0]++' 
	rm -f "$OUT/.linked"
	echo
	echo "## Further texts shipped inside those modules"
	echo
	echo "Required because the material is compiled in: \`modernc.org/libc\` selects its"
	echo "musl-translated implementation on linux/amd64 without a build tag."
	echo
	echo "| Component | Material | Text in this package |"
	echo "|---|---|---|"
	printf '%s\n' \
		'| modernc.org/libc | Go / musl / go-netdb / nixpkgs | licenses/modernc-libc-LICENSE-3RD-PARTY |' \
		'| modernc.org/sqlite | bundled third-party notices | licenses/modernc-sqlite-LICENSE-3RD-PARTY |' \
		'| modernc.org/sqlite | SQLite itself (public domain) | licenses/modernc-sqlite-LICENSE-SQLITE |' \
		'| modernc.org/memory | Go runtime material | licenses/modernc-memory-LICENSE-GO |' \
		'| modernc.org/memory | mmap-go material | licenses/modernc-memory-LICENSE-MMAP-GO |' \
		'| golang.org/x/sys | patent grant | licenses/x-sys-PATENTS |'
	echo
	echo "## Shipped upstream but not linked by this build"
	echo
	echo "| Component | Material | Text in this package |"
	echo "|---|---|---|"
	printf '%s\n' \
		'| modernc.org/sqlite | sqlite-vec | licenses/modernc-sqlite-LICENSE-SQLITE_VEC |' \
		'| modernc.org/memory | project logo reference (URL) | licenses/modernc-memory-LICENSE-LOGO |' \
		'| github.com/mattn/go-isatty | module requirement, not present in this build'"'"'s link set | licenses/go-isatty-LICENSE |' \
		'| github.com/ncruces/go-strftime | module requirement, not present in this build'"'"'s link set | licenses/go-strftime-LICENSE |'
	echo
	echo "## MPL-2.0"
	echo
	echo "\`github.com/go-sql-driver/mysql\` is covered by the Mozilla Public License 2.0."
	echo "Its complete text is licenses/go-sql-driver-mysql-LICENSE. How to obtain the"
	echo "corresponding source form of the MPL-covered files is described in"
	echo "MPL-2.0-SOURCE.md."
} >"$OUT/NOTICE.md"

cat >"$OUT/MPL-2.0-SOURCE.md" <<MPL
# Obtaining the source of the MPL-2.0 covered files

\`maskriver\` links \`github.com/go-sql-driver/mysql\` at the version pinned in this
release's \`go.mod\`: **v1.10.1**, which is covered by the Mozilla Public License 2.0.

The complete licence text is \`licenses/go-sql-driver-mysql-LICENSE\`.

To obtain the corresponding source form of the MPL-covered files:

\`\`\`sh
go mod download -x github.com/go-sql-driver/mysql@v1.10.1
# the module source is then at \$(go env GOMODCACHE)/github.com/go-sql-driver/mysql@v1.10.1
\`\`\`

The same version is obtainable from the upstream repository
\`https://github.com/go-sql-driver/mysql\` at tag \`v1.10.1\`.

MaskRiver does not modify those files: they are used as published by the module,
unmodified, through the Go module mechanism.
MPL

cat >"$OUT/BUILD-INFO.md" <<INFO
# Build information

| Field | Value |
|---|---|
| Project | MaskRiver |
| Version | $VERSION |
| Commit | $SHA |
| Platform | linux/amd64 |
| Go | $(go version | sed 's/^go version //') |
| Build mode | CGO_ENABLED=0, -trimpath, -ldflags "-s -w" |
| Frozen contract | APIRevision=m1a-v1 |

## Scope of use

Synthetic or otherwise authorised disposable SQLite snapshots only. Masking is not
anonymisation. Deterministic output is not collision-free. Batch transactions are
not cross-batch atomic and there is no exactly-once guarantee.

## Verified

SQLite multi-table CLI: \`scan\` → \`preview\` → \`approve\` (separate process) →
\`mask --apply\` in a new process → \`validate\`. A valid Ed25519 decision bound to the
whole plan is required in addition to \`--apply\` and \`--approved\`.

## Not verified — do not claim

MySQL CLI end to end · checkpoint or resume · exactly-once · production-grade
detection accuracy · macOS CI · independent strict semantics for \`--strict\` ·
MEMORY/OFF journal CLI paths · CLI-level cancellation · the remaining CN strategies ·
real concurrency snapshots.

## Known limitations

A rejected WAL source can still leave \`-shm\`/\`-wal\` sidecars, so no unconditional
zero-file-change promise is made. Decision records cannot detect whole-file rollback
or revocation rollback. \`internal/db\` still accepts a non-NULL → NULL change; the CLI
refuses it upstream. The trust file and the reviewer key are local deployment files:
an operator holding both is not prevented.
INFO

cd "$OUT"
find . -type f ! -name MANIFEST ! -name SHA256SUMS | sed 's|^\./||' | LC_ALL=C sort >MANIFEST
sha256sum $(cat MANIFEST) >SHA256SUMS

echo "package contents:"
sed 's/^/  /' MANIFEST
