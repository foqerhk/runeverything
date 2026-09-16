#!/usr/bin/env bash
# Alpine Linux: install release binary into /usr/local/bin (+ OpenRC note).
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-latest}"
PREFIX="${RE_PREFIX:-/usr/local}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Please run with sudo." >&2
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

echo
echo "Installed to ${PREFIX}/bin/runeverything"
echo "  runeverything pair"
echo "Alpine typically uses OpenRC; start manually or add your own service."
echo "APKBUILD template: packaging/alpine/APKBUILD"
