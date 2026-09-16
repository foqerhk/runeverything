#!/usr/bin/env bash
# Fedora/RHEL/CentOS/openSUSE: download .rpm and install via dnf/yum/zypper.
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-latest}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Please run with sudo." >&2
  exit 1
fi

pkg_mgr=""
if command -v dnf >/dev/null 2>&1; then
  pkg_mgr=dnf
elif command -v yum >/dev/null 2>&1; then
  pkg_mgr=yum
elif command -v zypper >/dev/null 2>&1; then
  pkg_mgr=zypper
else
  echo "dnf/yum/zypper not found. Use scripts/install.sh instead." >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) rpmarch=x86_64 ;;
  aarch64|arm64) rpmarch=aarch64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [[ -n "$tag" ]] || { echo "could not resolve latest release" >&2; exit 1; }
  ver="${tag#v}"
else
  ver="${VERSION#v}"
  tag="v${ver}"
fi

rpm="runeverything-${ver}-1.${rpmarch}.rpm"
url="https://github.com/${REPO}/releases/download/${tag}/${rpm}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "==> Downloading ${url}"
curl -fsSL "$url" -o "${tmpdir}/${rpm}"

echo "==> Installing with ${pkg_mgr}"
case "$pkg_mgr" in
  dnf|yum) "$pkg_mgr" install -y "${tmpdir}/${rpm}" ;;
  zypper) zypper --non-interactive install "${tmpdir}/${rpm}" ;;
esac

echo
echo "Installed. Next:"
echo "  runeverything pair"
echo "  systemctl --user enable --now runeverything   # optional"
