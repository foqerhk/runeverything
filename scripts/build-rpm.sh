#!/usr/bin/env bash
# Build .rpm packages (Fedora/RHEL/CentOS/openSUSE) via nfpm.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"

[[ -d "$OUT" ]] || { echo "missing ${OUT}; run build-release.sh first" >&2; exit 1; }

if ! command -v nfpm >/dev/null 2>&1; then
  echo "==> installing nfpm"
  GO_BIN="${GO_BIN:-}"
  if [[ -z "$GO_BIN" ]]; then
    if [[ -x "${HOME}/.local/go/bin/go" ]]; then
      GO_BIN="${HOME}/.local/go/bin/go"
    else
      GO_BIN="$(command -v go)"
    fi
  fi
  GOBIN="${HOME}/go/bin" "$GO_BIN" install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.41.3
  export PATH="${HOME}/go/bin:${PATH}"
fi

build_rpm() {
  local goarch="$1"
  local rpmarch="$2"
  local bin="${OUT}/runeverything_linux_${goarch}"
  [[ -f "$bin" ]] || { echo "missing ${bin}" >&2; return 1; }

  local cfg
  cfg="$(mktemp)"
  cat > "$cfg" <<EOF
name: runeverything
arch: ${rpmarch}
platform: linux
version: ${VERSION}
release: "1"
section: default
priority: optional
maintainer: foqerhk <123921693+foqerhk@users.noreply.github.com>
description: |
  Open-source remote agent for Intent Computing.
  Headless agent that pairs via QR code and exposes a PTY session through a relay.
vendor: foqerhk
homepage: https://github.com/foqerhk/runeverything
license: MIT
contents:
  - src: ${bin}
    dst: /usr/bin/runeverything
    file_info:
      mode: 0755
  - src: ${ROOT}/packaging/systemd/runeverything.service
    dst: /usr/lib/systemd/user/runeverything.service
EOF

  local rpmname="runeverything-${VERSION}-1.${rpmarch}.rpm"
  nfpm package --packager rpm --config "$cfg" --target "${OUT}/${rpmname}"
  rm -f "$cfg"
  echo "==> wrote ${OUT}/${rpmname}"
}

build_rpm amd64 x86_64
build_rpm arm64 aarch64
