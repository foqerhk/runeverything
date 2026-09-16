#!/usr/bin/env bash
# Debian/Ubuntu one-liner installer.
#
#   curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
#
# Order:
#   1) Launchpad PPA (ppa:foqerhk/runeverything) when available
#   2) GitHub Pages APT repo
#   3) Direct .deb from GitHub Releases
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-}"          # empty = install latest; or e.g. 0.1.0
PPA="${RE_PPA:-ppa:foqerhk/runeverything}"
APT_BASE="${RE_APT_BASE:-https://foqerhk.github.io/runeverything/apt}"
LIST_FILE="/etc/apt/sources.list.d/runeverything.list"
USE_PPA="${RE_USE_PPA:-1}"

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

install_pkg() {
  apt-get update -qq
  if [[ -n "$VERSION" ]]; then
    apt-get install -y "runeverything=${VERSION}*"
  else
    apt-get install -y runeverything
  fi
}

try_ppa() {
  [[ "$USE_PPA" == "1" ]] || return 1
  echo "==> Trying Launchpad PPA: ${PPA}"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq software-properties-common ca-certificates >/dev/null
  if ! add-apt-repository -y "$PPA"; then
    echo "==> add-apt-repository failed" >&2
    return 1
  fi
  install_pkg
}

add_pages_repo() {
  echo "==> Adding GitHub Pages APT source: ${APT_BASE}"
  echo "deb [trusted=yes arch=amd64,arm64] ${APT_BASE} stable main" > "${LIST_FILE}"
  chmod 0644 "${LIST_FILE}"
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

if try_ppa; then
  echo "==> Installed from Launchpad PPA"
elif add_pages_repo && install_pkg; then
  echo "==> Installed from GitHub Pages APT repository"
else
  echo "==> APT install failed; trying GitHub Release .deb"
  install_from_deb
fi

echo
echo "Done. You can now use:"
echo "  sudo apt upgrade runeverything"
echo "  runeverything pair"
echo "  systemctl --user enable --now runeverything   # optional"
