#!/usr/bin/env bash
# Build Windows GUI installer(s): RunEverythingSetup_<arch>.exe (agent embedded).
# Usage: ./scripts/build-windows-setup.sh <version> [amd64|arm64|386|all]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-dev}}"
ARCHES="${2:-all}"
OUT="${ROOT}/dist/windows"
mkdir -p "$OUT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found" >&2
  exit 1
fi

export CGO_ENABLED=0
# Prefer older Windows kernel compatibility where the toolchain allows.
# Note: Go 1.21+ officially requires Windows 10+; Win7 is not supported by the runtime.
export GOOS=windows
LDFLAGS="-s -w -X main.version=${VERSION}"

build_arch() {
  local goarch="$1"
  local tmp
  tmp="$(mktemp -d)"
  echo "==> windows/${goarch}: agent"
  GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything.exe" "${ROOT}/cmd/agent"

  mkdir -p "${ROOT}/cmd/winsetup/embedded"
  cp "${tmp}/runeverything.exe" "${ROOT}/cmd/winsetup/embedded/runeverything.exe"

  local setup_name="RunEverythingSetup_${goarch}.exe"
  echo "==> windows/${goarch}: ${setup_name}"
  GOARCH="$goarch" go build -trimpath -tags embedagent \
    -ldflags "${LDFLAGS} -H windowsgui" \
    -o "${OUT}/${setup_name}" "${ROOT}/cmd/winsetup"

  # Also keep raw agent next to setup for power users / scripts
  cp "${tmp}/runeverything.exe" "${OUT}/runeverything_windows_${goarch}.exe"
  rm -rf "$tmp"
  ls -la "${OUT}/${setup_name}"
}

case "$ARCHES" in
  all)
    build_arch amd64
    build_arch arm64
    build_arch 386
    ;;
  amd64|arm64|386)
    build_arch "$ARCHES"
    ;;
  *)
    echo "usage: $0 <version> [amd64|arm64|386|all]" >&2
    exit 2
    ;;
esac

echo
echo "Windows setups in ${OUT}:"
ls -la "$OUT"
echo
echo "Supported OS (Go toolchain): Windows 10 / 11 / Server 2016+ (amd64, arm64, 386)."
echo "Windows 7 / 8 are not supported by Go 1.21+; use a PC on Windows 10+."
