#!/usr/bin/env bash
# Arch Linux: install from release tarball (+ systemd user unit). Prefer AUR when published.
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-latest}"
PREFIX="${RE_PREFIX:-/usr}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Please run with sudo (writes to ${PREFIX})." >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64) goarch=amd64 ;;
  aarch64) goarch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [[ -n "$tag" ]] || { echo "could not resolve latest release" >&2; exit 1; }
else
  ver="${VERSION#v}"
  tag="v${ver}"
fi

url="https://github.com/${REPO}/releases/download/${tag}/runeverything_linux_${goarch}.tar.gz"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "==> Downloading ${url}"
curl -fsSL "$url" -o "${tmpdir}/re.tar.gz"
tar -C "$tmpdir" -xzf "${tmpdir}/re.tar.gz"

install -Dm755 "${tmpdir}/runeverything" "${PREFIX}/bin/runeverything"

unit_src=""
# Prefer unit shipped in repo raw content
if curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/runeverything.service" \
  -o "${tmpdir}/runeverything.service" 2>/dev/null; then
  unit_src="${tmpdir}/runeverything.service"
fi
if [[ -n "$unit_src" ]]; then
  install -Dm644 "$unit_src" /usr/lib/systemd/user/runeverything.service
  systemctl --user daemon-reload 2>/dev/null || true
fi

echo
echo "Installed to ${PREFIX}/bin/runeverything"
echo "  runeverything pair"
echo "  systemctl --user enable --now runeverything   # optional"
echo
echo "AUR-style PKGBUILD is also available in the repo under packaging/aur/"
