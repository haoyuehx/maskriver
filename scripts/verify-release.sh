#!/usr/bin/env bash
# Verify a MaskRiver release archive and its package contents.
#
# Usage: scripts/verify-release.sh <archive> [--version V] [--commit SHA] [--sha256 HEX]
set -euo pipefail

ARCHIVE=${1:?usage: verify-release.sh <archive> --sha256 HEX [--version V] [--commit SHA] [--allow-untagged]}
shift

EXPECTED_VERSION=""
EXPECTED_COMMIT=""
EXPECTED_SHA=""
ALLOW_UNTAGGED=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) EXPECTED_VERSION=${2:?}; shift 2 ;;
    --commit) EXPECTED_COMMIT=${2:?}; shift 2 ;;
    --sha256) EXPECTED_SHA=${2:?}; shift 2 ;;
    --allow-untagged) ALLOW_UNTAGGED=1; shift ;;
    *) echo "verify-release: unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ -f "$ARCHIVE" ]] || { echo "verify-release: archive not found: $ARCHIVE" >&2; exit 1; }
[[ -n "$EXPECTED_SHA" ]] || { echo "verify-release: --sha256 is required (do not execute an unauthenticated archive)" >&2; exit 1; }
ARCHIVE=$(cd "$(dirname "$ARCHIVE")" && pwd)/$(basename "$ARCHIVE")
ACTUAL_SHA=$(sha256sum "$ARCHIVE" | cut -d' ' -f1)
if [[ -n "$EXPECTED_SHA" && "$ACTUAL_SHA" != "$EXPECTED_SHA" ]]; then
  echo "verify-release: archive SHA256 mismatch: got $ACTUAL_SHA want $EXPECTED_SHA" >&2
  exit 1
fi

if [[ -z "$EXPECTED_VERSION" ]]; then
  EXPECTED_VERSION=$(basename "$ARCHIVE" | sed -E 's/^maskriver-v(.*)-linux-amd64\.tar\.gz$/\1/')
fi
[[ -n "$EXPECTED_VERSION" ]] || { echo "verify-release: cannot derive version from archive name; pass --version" >&2; exit 1; }
VERSION=$EXPECTED_VERSION

WORK=$(mktemp -d)
[[ -n "$WORK" && "$WORK" != "/" ]] || { echo "verify-release: unsafe temp dir" >&2; exit 1; }
trap 'rm -rf "$WORK"' EXIT
export LC_ALL=C TZ=UTC
python3 - "$ARCHIVE" "$VERSION" <<'PY'
import sys, tarfile
archive, version = sys.argv[1], sys.argv[2]
expected = f"maskriver-v{version}-linux-amd64"
tops = set()
with tarfile.open(archive, "r:gz") as tf:
    for member in tf.getmembers():
        name = member.name
        if name.startswith("/") or name.startswith("../") or "/../" in name:
            raise SystemExit(f"verify-release: unsafe archive member: {name}")
        if member.issym() or member.islnk() or not (member.isdir() or member.isfile()):
            raise SystemExit(f"verify-release: non-regular archive member: {name}")
        tops.add(name.split("/", 1)[0])
if tops != {expected}:
    raise SystemExit(f"verify-release: top-level entries {sorted(tops)} != {[expected]}")
PY
tar -xzf "$ARCHIVE" -C "$WORK"
mapfile -t ROOT_ENTRIES < <(find "$WORK" -mindepth 1 -maxdepth 1 -printf '%f\n')
if [[ ${#ROOT_ENTRIES[@]} -ne 1 ]]; then
  echo "verify-release: archive must contain exactly one top-level entry" >&2
  exit 1
fi
PKG="$WORK/${ROOT_ENTRIES[0]}"

required=(maskriver LICENSE NOTICE.md MPL-2.0-SOURCE.md BUILD-INFO.md MANIFEST SHA256SUMS README.md RELEASE-NOTES.md SBOM.json licenses/go-toolchain-LICENSE)
for file in "${required[@]}"; do
  [[ -f "$PKG/$file" ]] || { echo "verify-release: missing required file: $file" >&2; exit 1; }
done
[[ -d "$PKG/licenses" ]] || { echo "verify-release: missing licenses/ directory" >&2; exit 1; }
[[ -x "$PKG/maskriver" ]] || { echo "verify-release: binary is not executable" >&2; exit 1; }

# No forbidden material.
if find "$PKG" \( -name '.git' -o -name '.env*' -o -name '*.db' -o -name '*.sqlite*' \
     -o -name '*.key' -o -name '*.seed' -o -name 'id_ed25519*' -o -name '*.pem' \) -print -quit | grep -q .; then
  echo "verify-release: forbidden file material found" >&2
  exit 1
fi

# MANIFEST and checksums must describe exactly the package contents.
(
  cd "$PKG"
  sha256sum -c --strict SHA256SUMS >/dev/null
  awk '{print $2}' SHA256SUMS | LC_ALL=C sort >"$WORK/sha-files"
  LC_ALL=C sort MANIFEST >"$WORK/manifest-files"
  if ! diff -u "$WORK/manifest-files" "$WORK/sha-files" >"$WORK/sums.diff"; then
    echo "verify-release: SHA256SUMS does not cover exactly MANIFEST" >&2
    cat "$WORK/sums.diff" >&2
    exit 1
  fi
)
(
  cd "$PKG"
  find . -type f ! -name MANIFEST ! -name SHA256SUMS | sed 's|^\./||' | LC_ALL=C sort >"$WORK/actual-manifest"
  LC_ALL=C sort MANIFEST >"$WORK/declared-manifest"
  if ! diff -u "$WORK/declared-manifest" "$WORK/actual-manifest" >"$WORK/manifest.diff"; then
    echo "verify-release: MANIFEST does not match package contents" >&2
    cat "$WORK/manifest.diff" >&2
    exit 1
  fi
)

# Every licence reference in NOTICE must resolve.
while read -r ref; do
  [[ -f "$PKG/$ref" ]] || { echo "verify-release: dangling NOTICE reference: $ref" >&2; exit 1; }
done < <(grep -oE 'licenses/[A-Za-z0-9._-]*[A-Za-z0-9_-]' "$PKG/NOTICE.md" | sort -u)
# Every licences/ file must be referenced by NOTICE (no unexplained files).
for file in "$PKG"/licenses/*; do
  base=$(basename "$file")
  grep -Eq "(^|[^A-Za-z0-9._/-])licenses/$base($|[^A-Za-z0-9._/-])" "$PKG/NOTICE.md" || {
    echo "verify-release: licence file not referenced by NOTICE: $base" >&2
    exit 1
  }
done

# Version and provenance.
VERSION=$(sed -n 's/^| Version | \(.*\) |$/\1/p' "$PKG/BUILD-INFO.md")
COMMIT=$(sed -n 's/^| Commit | \(.*\) |$/\1/p' "$PKG/BUILD-INFO.md")
[[ -n "$VERSION" && -n "$COMMIT" ]] || { echo "verify-release: BUILD-INFO has no version/commit" >&2; exit 1; }
if [[ -n "$EXPECTED_VERSION" && "$VERSION" != "$EXPECTED_VERSION" ]]; then
  echo "verify-release: BUILD-INFO version $VERSION != $EXPECTED_VERSION" >&2
  exit 1
fi
if [[ -n "$EXPECTED_COMMIT" && "$COMMIT" != "$EXPECTED_COMMIT" ]]; then
  echo "verify-release: BUILD-INFO commit $COMMIT != $EXPECTED_COMMIT" >&2
  exit 1
fi
OUTPUT=$("$PKG/maskriver" --version)
if [[ "$OUTPUT" != "MaskRiver $VERSION" ]]; then
  echo "verify-release: --version output '$OUTPUT' does not match version $VERSION" >&2
  exit 1
fi
"$PKG/maskriver" --help >/dev/null

go version -m "$PKG/maskriver" >"$WORK/goversion.txt"
grep -q "vcs.revision=$COMMIT$" "$WORK/goversion.txt" || {
  echo "verify-release: binary vcs.revision does not match $COMMIT" >&2; exit 1;
}
grep -q "vcs.modified=false$" "$WORK/goversion.txt" || {
  echo "verify-release: binary vcs.modified is not false" >&2; exit 1;
}
grep -q "GOOS=linux$" "$WORK/goversion.txt" || { echo "verify-release: not linux" >&2; exit 1; }
grep -q "GOARCH=amd64$" "$WORK/goversion.txt" || { echo "verify-release: not amd64" >&2; exit 1; }
GO_VERSION=$(sed -n 's/^| Go | \(.*\) |$/\1/p' "$PKG/BUILD-INFO.md")
BINARY_GO=$(head -1 "$WORK/goversion.txt" | sed -E 's/.*: (go[0-9][^ ]*).*/\1/')
if [[ -n "$GO_VERSION" && "$GO_VERSION" != "$BINARY_GO" ]]; then
  echo "verify-release: BUILD-INFO Go $GO_VERSION != binary $BINARY_GO" >&2
  exit 1
fi

# SBOM must list the same modules as the binary metadata.
python3 - "$PKG/SBOM.json" "$WORK/goversion.txt" "$PKG/NOTICE.md" "$PKG/BUILD-INFO.md" <<PY
import json, re, sys
sbom = json.load(open(sys.argv[1]))
text = open(sys.argv[2]).read()
notice = open(sys.argv[3]).read()
build_info = open(sys.argv[4]).read()

def info(field):
    m = re.search(r"^\| " + re.escape(field) + r" \| (.*?) \|$", build_info, re.M)
    return m.group(1) if m else None

if sbom.get("version") != info("Version") or sbom.get("commit") != info("Commit") or \
   sbom.get("go") != info("Go") or sbom.get("platform") != "linux/amd64":
    print("verify-release: SBOM header does not match BUILD-INFO", file=sys.stderr)
    sys.exit(1)

sbom_mods = {m["path"] for m in sbom["modules"]}
binary = {}
for line in text.splitlines():
    m = re.match(r"^\tdep\t([^\t]+)\t([^\t]+)\t([^\t]+)", line)
    if m:
        binary[m.group(1)] = (m.group(2), m.group(3))
binary_mods = set(binary)
if sbom_mods != binary_mods:
    print("verify-release: SBOM/binary module set mismatch", file=sys.stderr)
    print("  only SBOM:", sorted(sbom_mods - binary_mods), file=sys.stderr)
    print("  only binary:", sorted(binary_mods - sbom_mods), file=sys.stderr)
    sys.exit(1)
for module in sbom["modules"]:
    path = module["path"]
    want = (module.get("version"), module.get("sum"))
    got = binary[path]
    if want[0] != got[0] or want[1] != got[1]:
        print(f"verify-release: SBOM entry for {path} {want} != binary {got}", file=sys.stderr)
        sys.exit(1)

notice_mods = set()
in_table = False
for line in notice.splitlines():
    if line.strip() == "## Modules linked into the binary":
        in_table = True
        continue
    if in_table:
        if line.startswith("## "):
            break
        if line.startswith("| ") and not line.startswith("| Module ") and not line.startswith("|---"):
            notice_mods.add(line.split("|")[1].strip())
if notice_mods != binary_mods:
    print("verify-release: NOTICE/binary module set mismatch", file=sys.stderr)
    print("  only NOTICE:", sorted(notice_mods - binary_mods), file=sys.stderr)
    print("  only binary:", sorted(binary_mods - notice_mods), file=sys.stderr)
    sys.exit(1)

# The main module version is the release tag when built from a tagged commit.
if not ${ALLOW_UNTAGGED:-0}:
    main_mod = None
    for line in text.splitlines():
        m = re.match(r"^\tmod\tgithub.com/haoyuehx/maskriver\t(\S+)", line)
        if m:
            main_mod = m.group(1)
    if main_mod != "v$VERSION":
        print(f"verify-release: binary main-module version {main_mod!r} is not v$VERSION; build from the release tag", file=sys.stderr)
        sys.exit(1)
PY

echo "verified: $(basename "$ARCHIVE")"
echo "version: $VERSION"
echo "commit: $COMMIT"
echo "sha256: $ACTUAL_SHA"
echo "size: $(stat -c '%s' "$ARCHIVE") bytes"
