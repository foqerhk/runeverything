#!/usr/bin/env bash
# Cross-compile Agent release archives for Homebrew / install.sh / GitHub Releases.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
mkdir -p "$OUT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found (need Go 1.24+)" >&2
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
  echo "==> building ${name}"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything${ext}" "${ROOT}/cmd/agent"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-relay${ext}" "${ROOT}/cmd/relay"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" \
    -o "${tmp}/runeverything-directory${ext}" "${ROOT}/cmd/directory"

  if [[ "$goos" == "windows" ]]; then
    (cd "$tmp" && zip -q "${OUT}/${name}.zip" "runeverything${ext}" "runeverything-relay${ext}" "runeverything-directory${ext}")
  else
    tar -C "$tmp" -czf "${OUT}/${name}.tar.gz" "runeverything${ext}" "runeverything-relay${ext}" "runeverything-directory${ext}"
  fi
  # Also keep raw agent binary for install.sh fallback naming
  cp "${tmp}/runeverything${ext}" "${OUT}/${name}${ext}"
  cp "${tmp}/runeverything-relay${ext}" "${OUT}/runeverything-relay_${goos}_${goarch}${ext}"
  rm -rf "$tmp"
}

cd "$ROOT"
build_one darwin amd64
build_one darwin arm64
build_one linux amd64
build_one linux arm64
build_one windows amd64
build_one windows arm64

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
echo "Next:"
echo "  1. Create git tag v${VERSION} and push"
echo "  2. Upload ${OUT}/* to GitHub Release v${VERSION}"
echo "  3. Run sync scripts + publish APT Pages if needed"
echo "  4. Debian/Ubuntu: curl -fsSL .../scripts/install-apt.sh | sudo bash"
