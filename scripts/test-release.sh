#!/usr/bin/env bash
# Build the same release twice in independent output roots, compare bytes and
# verify the archive. Intended for CI and release rehearsal.
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO"
export LC_ALL=C TZ=UTC

VERSION=$(sed -n 's/^const version = "\(.*\)"/\1/p' cmd/maskriver/main.go)
[[ -n "$VERSION" ]] || { echo "test-release: missing version" >&2; exit 1; }
SHA=$(git rev-parse HEAD)

ROOT=$(mktemp -d)
[[ -n "$ROOT" && "$ROOT" != "/" ]] || { echo "test-release: unsafe temp dir" >&2; exit 1; }
trap '[[ "${MASKRIVER_RELEASE_TEST_KEEP:-0}" == "1" ]] || rm -rf "$ROOT"' EXIT
mkdir -p "$ROOT/a" "$ROOT/b"
scripts/build-release.sh "$ROOT/a" >/dev/null
scripts/build-release.sh "$ROOT/b" >/dev/null
A="$ROOT/a/maskriver-v$VERSION-linux-amd64.tar.gz"
B="$ROOT/b/maskriver-v$VERSION-linux-amd64.tar.gz"
SHA_A=$(sha256sum "$A" | cut -d' ' -f1)
SHA_B=$(sha256sum "$B" | cut -d' ' -f1)
if [[ "$SHA_A" != "$SHA_B" ]]; then
  echo "test-release: archives are not reproducible: $SHA_A != $SHA_B" >&2
  cmp -l "$A" "$B" | head -20 >&2 || true
  exit 1
fi
scripts/verify-release.sh "$A" --version "$VERSION" --commit "$SHA" --sha256 "$SHA_A" --allow-untagged
echo "reproducible archive: $SHA_A"
