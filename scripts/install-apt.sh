#!/usr/bin/env bash
# Debian/Ubuntu one-liner installer.
#
#   curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
#
# This script:
#   1) Adds our APT repository (so later you can: sudo apt install/upgrade runeverything)
#   2) Runs apt update && apt install runeverything
#   3) Falls back to downloading the .deb from GitHub Releases if the repo is unreachable
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-}"          # empty = install from APT repo "latest"; or e.g. 0.1.0
APT_BASE="${RE_APT_BASE:-https://foqerhk.github.io/runeverything/apt}"
LIST_FILE="/etc/apt/sources.list.d/runeverything.list"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Please run with sudo, e.g.:" >&2
  echo "  curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash" >&2
  exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
  echo "apt-get not found. Use scripts/install-linux.sh or scripts/install.sh instead." >&2
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

add_apt_repo() {
  echo "==> Adding APT source: ${APT_BASE}"
  # trusted=yes: unsigned GitHub Pages repo (fine for early OSS; swap for signed key later)
  echo "deb [trusted=yes arch=amd64,arm64] ${APT_BASE} stable main" > "${LIST_FILE}"
  chmod 0644 "${LIST_FILE}"
}

install_from_repo() {
  apt-get update -qq
  if [[ -n "$VERSION" ]]; then
    apt-get install -y "runeverything=${VERSION}"
  else
    apt-get install -y runeverything
  fi
}

install_from_deb() {
  local ver tag base deb url tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' RETURN

  if [[ -n "$VERSION" ]]; then
    ver="${VERSION#v}"
    tag="v${ver}"
  else
    tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
    [[ -n "$tag" ]] || { echo "could not resolve latest release" >&2; return 1; }
    ver="${tag#v}"
  fi

  base="https://github.com/${REPO}/releases/download/${tag}"
  deb="runeverything_${ver}_${arch}.deb"
  url="${base}/${deb}"
  echo "==> Fallback: downloading ${url}"
  curl -fsSL "$url" -o "${tmpdir}/${deb}"
  apt-get install -y "${tmpdir}/${deb}"
}

add_apt_repo

echo "==> Installing runeverything via apt"
if install_from_repo; then
  echo "==> Installed from APT repository"
else
  echo "==> APT repo install failed; trying GitHub Release .deb"
  install_from_deb
fi

echo
echo "Done. You can now use:"
echo "  sudo apt install runeverything     # already installed"
echo "  sudo apt upgrade runeverything     # later upgrades (source already added)"
echo "  runeverything pair"
echo "  systemctl --user enable --now runeverything   # optional"
