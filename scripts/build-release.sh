#!/usr/bin/env bash
# Cross-compile Agent release archives for Homebrew / install.sh / GitHub Releases.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
mkdir -p "$OUT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found (need Go 1.25+)" >&2
  exit 1
fi

export CGO_ENABLED=0
LDFLAGS="-s -w -X main.version=${VERSION}"

build_one() {
  local goos="$1" goarch="$2"
  local name="runeverything_${goos}_${goarch}"
  local ext=""
  [[ "$goos" == "windows" ]] && ext=".exe"
  local tmp
  tmp="$(mktemp -d)"
  # Desktop capture on macOS needs Apple frameworks (CGO).
  local cgo=0
  [[ "$goos" == "darwin" ]] && cgo=1
  echo "==> building ${name} (CGO_ENABLED=${cgo})"
  CGO_ENABLED="$cgo" GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything${ext}" "${ROOT}/cmd/agent"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-relay${ext}" "${ROOT}/cmd/relay"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-directory${ext}" "${ROOT}/cmd/directory"

  if [[ "$goos" == "windows" ]]; then
    # One-file GUI installer for end users (embeds agent).
    mkdir -p "${ROOT}/cmd/winsetup/embedded"
    cp "${tmp}/runeverything${ext}" "${ROOT}/cmd/winsetup/embedded/runeverything.exe"
    local setup_name="RunEverythingSetup_${goarch}.exe"
    echo "==> building ${setup_name}"
    GOOS=windows GOARCH="$goarch" go build -trimpath -tags embedagent \
      -ldflags "${LDFLAGS} -H windowsgui" \
      -o "${OUT}/${setup_name}" "${ROOT}/cmd/winsetup"
    (cd "$tmp" && zip -q "${OUT}/${name}.zip" "runeverything${ext}" "runeverything-relay${ext}" "runeverything-directory${ext}")
  else
    tar -C "$tmp" -czf "${OUT}/${name}.tar.gz" "runeverything${ext}" "runeverything-relay${ext}" "runeverything-directory${ext}"
  fi
  cp "${tmp}/runeverything${ext}" "${OUT}/${name}${ext}"
  cp "${tmp}/runeverything-relay${ext}" "${OUT}/runeverything-relay_${goos}_${goarch}${ext}"
  rm -rf "$tmp"
}

cd "$ROOT"
# Darwin needs Apple CGO — skip on Linux CI; use scripts/build-darwin.sh / macos runners.
if [[ "$(uname -s)" == "Darwin" ]]; then
  build_one darwin amd64
  build_one darwin arm64
else
  echo "==> skip darwin cross-compile on $(uname -s) (use macOS runner / build-darwin.sh)"
fi
build_one linux amd64
build_one linux arm64
build_one windows amd64
build_one windows arm64
build_one windows 386

echo "==> building .deb packages"
chmod +x "${ROOT}/scripts/build-deb.sh"
"${ROOT}/scripts/build-deb.sh" "$VERSION"

echo "==> building .rpm packages"
chmod +x "${ROOT}/scripts/build-rpm.sh"
"${ROOT}/scripts/build-rpm.sh" "$VERSION" || echo "warning: rpm build failed (install nfpm / try again)"

if [[ "${RE_SKIP_APT_REPO:-}" != "1" ]]; then
  echo "==> building APT repo tree"
  chmod +x "${ROOT}/scripts/build-apt-repo.sh"
  "${ROOT}/scripts/build-apt-repo.sh" "$VERSION" || echo "warning: apt repo build failed (non-fatal)"
fi

echo "==> checksums"
(
  cd "$OUT"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 ./* > SHA256SUMS
  else
    sha256sum ./* > SHA256SUMS
  fi
)

echo
echo "Artifacts in ${OUT}"
ls -la "$OUT"
echo
echo "Windows users: download RunEverythingSetup_amd64.exe (or arm64) from the GitHub Release."
