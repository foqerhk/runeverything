#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
PB="${ROOT}/packaging/aur/PKGBUILD"

sha_of() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

AMD=$(sha_of "${OUT}/runeverything_linux_amd64.tar.gz")
ARM=$(sha_of "${OUT}/runeverything_linux_arm64.tar.gz")

perl -i -pe "s/^pkgver=.*/pkgver=${VERSION}/" "$PB"
perl -i -pe "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('${AMD}')/" "$PB"
perl -i -pe "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('${ARM}')/" "$PB"
echo "updated ${PB}"
