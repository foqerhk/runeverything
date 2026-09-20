#!/usr/bin/env bash
# Build a simple APT repository tree from .deb files (for GitHub Pages).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
REPO="${ROOT}/dist/apt"

rm -rf "$REPO"
mkdir -p "${REPO}/pool/main" "${REPO}/dists/stable/main/binary-amd64" "${REPO}/dists/stable/main/binary-arm64"

cp "${OUT}/runeverything_${VERSION}_amd64.deb" "${REPO}/pool/main/"
cp "${OUT}/runeverything_${VERSION}_arm64.deb" "${REPO}/pool/main/"

gen_packages() {
  local arch="$1"
  local dest="$2"
  local deb="${REPO}/pool/main/runeverything_${VERSION}_${arch}.deb"
  local size
  size="$(wc -c < "$deb" | tr -d ' ')"
  local sha256 md5
  if command -v shasum >/dev/null 2>&1; then
    sha256="$(shasum -a 256 "$deb" | awk '{print $1}')"
    md5="$(md5 -q "$deb" 2>/dev/null || md5sum "$deb" | awk '{print $1}')"
  else
    sha256="$(sha256sum "$deb" | awk '{print $1}')"
    md5="$(md5sum "$deb" | awk '{print $1}')"
  fi

  # Extract control fields
  local tmp
  tmp="$(mktemp -d)"
  if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb -e "$deb" "$tmp"
  else
    (
      cd "$tmp"
      ar x "$deb" control.tar.gz 2>/dev/null || ar -x "$deb" control.tar.gz
      tar -xzf control.tar.gz
    )
  fi

  {
    cat "${tmp}/control"
    echo "Filename: pool/main/runeverything_${VERSION}_${arch}.deb"
    echo "Size: ${size}"
    echo "MD5sum: ${md5}"
    echo "SHA256: ${sha256}"
    echo
  } > "${dest}/Packages"
  gzip -9c "${dest}/Packages" > "${dest}/Packages.gz"
  rm -rf "$tmp"
}

gen_packages amd64 "${REPO}/dists/stable/main/binary-amd64"
gen_packages arm64 "${REPO}/dists/stable/main/binary-arm64"

# Release file
{
  echo "Origin: RunEverything"
  echo "Label: RunEverything"
  echo "Suite: stable"
  echo "Codename: stable"
  echo "Architectures: amd64 arm64"
  echo "Components: main"
  echo "Description: RunEverything APT repository"
  echo "Date: $(date -u '+%a, %d %b %Y %H:%M:%S UTC')"
} > "${REPO}/dists/stable/Release"

# Append checksums for Packages files
(
  cd "${REPO}/dists/stable"
  echo "MD5Sum:"
  for f in main/binary-amd64/Packages main/binary-amd64/Packages.gz \
           main/binary-arm64/Packages main/binary-arm64/Packages.gz; do
    if command -v md5 >/dev/null 2>&1; then
      printf " %s %8d %s\n" "$(md5 -q "$f")" "$(wc -c < "$f" | tr -d ' ')" "$f"
    else
      printf " %s %8d %s\n" "$(md5sum "$f" | awk '{print $1}')" "$(wc -c < "$f" | tr -d ' ')" "$f"
    fi
  done
  echo "SHA256:"
  for f in main/binary-amd64/Packages main/binary-amd64/Packages.gz \
           main/binary-arm64/Packages main/binary-arm64/Packages.gz; do
    if command -v shasum >/dev/null 2>&1; then
      printf " %s %8d %s\n" "$(shasum -a 256 "$f" | awk '{print $1}')" "$(wc -c < "$f" | tr -d ' ')" "$f"
    else
      printf " %s %8d %s\n" "$(sha256sum "$f" | awk '{print $1}')" "$(wc -c < "$f" | tr -d ' ')" "$f"
    fi
  done
) >> "${REPO}/dists/stable/Release"

# Helper index for humans
cat > "${REPO}/README.md" <<EOF
# RunEverything APT repository

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
\`\`\`

Or add the source manually (after publishing this tree to GitHub Pages):

\`\`\`bash
echo 'deb [trusted=yes] https://foqerhk.github.io/runeverything/apt stable main' | sudo tee /etc/apt/sources.list.d/runeverything.list
sudo apt update
sudo apt install runeverything
\`\`\`
EOF

echo "==> APT repo at ${REPO}"
