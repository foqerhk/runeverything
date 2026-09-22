#!/usr/bin/env bash
# Build macOS agent/relay tarballs (requires macOS + CGO for ScreenCaptureKit / AppKit).
# Usage: ./scripts/build-darwin.sh <version> [arm64|amd64|all]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-dev}}"
ARCHES="${2:-all}"
OUT="${ROOT}/dist/v${VERSION}"
mkdir -p "$OUT"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: build-darwin.sh must run on macOS (CGO + Apple frameworks)" >&2
  exit 1
fi

export CGO_ENABLED=1
export GOOS=darwin
LDFLAGS="-s -w -X main.version=${VERSION}"

build_arch() {
  local goarch="$1"
  local tmp name
  tmp="$(mktemp -d)"
  name="runeverything_darwin_${goarch}"
  echo "==> ${name} (CGO=1)"
  GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything" "${ROOT}/cmd/agent"
  GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-relay" "${ROOT}/cmd/relay"
  GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-directory" "${ROOT}/cmd/directory" || true
  tar -C "$tmp" -czf "${OUT}/${name}.tar.gz" runeverything runeverything-relay \
    $( [[ -f "${tmp}/runeverything-directory" ]] && echo runeverything-directory )
  cp "${tmp}/runeverything" "${OUT}/${name}"
  rm -rf "$tmp"
  ls -la "${OUT}/${name}.tar.gz"
}

case "$ARCHES" in
  all) build_arch arm64; build_arch amd64 ;;
  arm64|amd64) build_arch "$ARCHES" ;;
  *) echo "usage: $0 <version> [arm64|amd64|all]" >&2; exit 2 ;;
esac
