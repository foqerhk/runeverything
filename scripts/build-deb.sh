#!/usr/bin/env bash
# Build .deb packages for Debian/Ubuntu (amd64, arm64) from linux binaries in dist/.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"

[[ -d "$OUT" ]] || { echo "missing ${OUT}; run build-release.sh first" >&2; exit 1; }

build_deb() {
  local goarch="$1"
  local debarch="$2"
  local bin="${OUT}/runeverything_linux_${goarch}"
  [[ -f "$bin" ]] || { echo "missing binary ${bin}" >&2; return 1; }

  local stage
  stage="$(mktemp -d)"
  mkdir -p "${stage}/DEBIAN" "${stage}/usr/bin" \
    "${stage}/lib/systemd/user" "${stage}/usr/share/doc/runeverything"

  install -m 0755 "$bin" "${stage}/usr/bin/runeverything"

  cat > "${stage}/lib/systemd/user/runeverything.service" <<EOF
[Unit]
Description=RunEverything Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/runeverything run -no-qr
Restart=on-failure
RestartSec=3
Environment=RE_HOME=%h/.runeverything

[Install]
WantedBy=default.target
EOF

  cat > "${stage}/usr/share/doc/runeverything/copyright" <<EOF
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: runeverything
Source: https://github.com/foqerhk/runeverything

Files: *
Copyright: 2026 foqerhk
License: MIT
EOF

  cat > "${stage}/usr/share/doc/runeverything/changelog.Debian" <<EOF
runeverything (${VERSION}) stable; urgency=medium

  * Package release ${VERSION}.

 -- foqerhk <123921693+foqerhk@users.noreply.github.com>  $(date -u '+%a, %d %b %Y %H:%M:%S +0000')
EOF
  gzip -9n "${stage}/usr/share/doc/runeverything/changelog.Debian"

  local size
  size="$(du -sk "${stage}/usr" | awk '{print $1}')"

  cat > "${stage}/DEBIAN/control" <<EOF
Package: runeverything
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${debarch}
Maintainer: foqerhk <123921693+foqerhk@users.noreply.github.com>
Installed-Size: ${size}
Homepage: https://github.com/foqerhk/runeverything
Description: Open-source remote agent for Intent Computing
 Headless agent that pairs via QR code and exposes a PTY session
 through a relay for Intent Computing clients.
EOF

  cat > "${stage}/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload >/dev/null 2>&1 || true
fi
echo "RunEverything installed. Pair with: runeverything pair"
echo "Optional user service: systemctl --user enable --now runeverything"
exit 0
EOF
  chmod 0755 "${stage}/DEBIAN/postinst"

  local debname="runeverything_${VERSION}_${debarch}.deb"
  local debpath="${OUT}/${debname}"

  if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --build --root-owner-group "$stage" "$debpath"
  else
    # Portable .deb builder (macOS BSD tar / ar)
    local work
    work="$(mktemp -d)"
    (
      cd "${stage}/DEBIAN"
      if tar --version 2>&1 | head -1 | grep -qi gnu; then
        tar --owner=0 --group=0 -czf "${work}/control.tar.gz" .
      else
        tar -czf "${work}/control.tar.gz" .
      fi
    )
    (
      cd "$stage"
      # Pack everything except DEBIAN/
      if tar --version 2>&1 | head -1 | grep -qi gnu; then
        tar --owner=0 --group=0 --exclude='./DEBIAN' -czf "${work}/data.tar.gz" .
      else
        tar -czf "${work}/data.tar.gz" --exclude='DEBIAN' .
      fi
    )
    printf '2.0\n' > "${work}/debian-binary"
    rm -f "$debpath"
    (
      cd "$work"
      ar -rc "$debpath" debian-binary control.tar.gz data.tar.gz
    )
    rm -rf "$work"
  fi

  rm -rf "$stage"
  echo "==> wrote ${debpath}"
}

build_deb amd64 amd64
build_deb arm64 arm64
