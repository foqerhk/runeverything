#!/usr/bin/env bash
# Debian/Ubuntu one-liner: download .deb and install via apt.
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-latest}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Please run with sudo (apt needs root)." >&2
  exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
  echo "apt-get not found. Use scripts/install.sh on non-Debian systems." >&2
  exit 1
fi

arch="$(dpkg --print-architecture 2>/dev/null || true)"
case "$arch" in
  amd64|arm64) ;;
  *)
    echo "unsupported dpkg architecture: ${arch:-unknown}" >&2
    exit 1
    ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if [[ "$VERSION" == "latest" ]]; then
  # Resolve latest tag via GitHub API redirect / releases/latest
  api="https://api.github.com/repos/${REPO}/releases/latest"
  tag="$(curl -fsSL "$api" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [[ -n "$tag" ]] || { echo "could not resolve latest release" >&2; exit 1; }
  ver="${tag#v}"
  base="https://github.com/${REPO}/releases/download/${tag}"
else
  ver="${VERSION#v}"
  tag="v${ver}"
  base="https://github.com/${REPO}/releases/download/${tag}"
fi

deb="runeverything_${ver}_${arch}.deb"
url="${base}/${deb}"
echo "==> Downloading ${url}"
curl -fsSL "$url" -o "${tmpdir}/${deb}"

echo "==> Installing with apt"
apt-get update -qq || true
apt-get install -y "${tmpdir}/${deb}"

echo
echo "Installed. Next:"
echo "  runeverything pair"
echo "  systemctl --user enable --now runeverything   # optional"
