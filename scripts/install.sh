#!/usr/bin/env bash
# RunEverything one-line installer (macOS / Linux)
set -euo pipefail

REPO="${RE_REPO:-foqerhk/runeverything}"
VERSION="${RE_VERSION:-latest}"
INSTALL_DIR="${RE_INSTALL_DIR:-${HOME}/.local/bin}"
RELAY_URL="${RE_RELAY:-ws://127.0.0.1:8787/ws}"
PREFIX_NAME="runeverything"

info() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) die "unsupported arch: $arch" ;;
esac
case "$os" in
  darwin|linux) ;;
  *) die "unsupported OS: $os (use install.ps1 on Windows)" ;;
esac

mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/${PREFIX_NAME}"

# Prefer local build if present next to this script's repo.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
if [[ -x "${REPO_ROOT}/bin/runeverything" ]]; then
  info "Installing local binary from ${REPO_ROOT}/bin/runeverything"
  cp "${REPO_ROOT}/bin/runeverything" "$TARGET"
  chmod +x "$TARGET"
elif command -v go >/dev/null 2>&1 && [[ -f "${REPO_ROOT}/go.mod" ]]; then
  info "Building from source at ${REPO_ROOT}"
  (cd "$REPO_ROOT" && go build -o "$TARGET" ./cmd/agent)
else
  # Prefer .tar.gz from GitHub Releases (same assets as Homebrew).
  local_base=""
  if [[ "$VERSION" == "latest" ]]; then
    local_base="https://github.com/${REPO}/releases/latest/download"
  else
    ver="$VERSION"
    [[ "$ver" != v* ]] && ver="v${ver}"
    local_base="https://github.com/${REPO}/releases/download/${ver}"
  fi
  archive="runeverything_${os}_${arch}.tar.gz"
  URL="${local_base}/${archive}"
  info "Downloading ${URL}"
  tmpdir="$(mktemp -d)"
  if ! curl -fsSL "$URL" -o "${tmpdir}/${archive}"; then
    # Fallback: raw binary name (older releases)
    raw_url="${local_base}/runeverything_${os}_${arch}"
    info "tar.gz missing, trying ${raw_url}"
    if ! curl -fsSL "$raw_url" -o "$TARGET"; then
      rm -rf "$tmpdir"
      die "download failed; build locally or set RE_REPO/RE_VERSION"
    fi
    chmod +x "$TARGET"
    rm -rf "$tmpdir"
  else
    tar -C "$tmpdir" -xzf "${tmpdir}/${archive}"
    mv "${tmpdir}/runeverything" "$TARGET"
    chmod +x "$TARGET"
    rm -rf "$tmpdir"
  fi
fi

# Persist default relay
mkdir -p "${HOME}/.runeverything"
if [[ ! -f "${HOME}/.runeverything/config.json" ]]; then
  cat > "${HOME}/.runeverything/config.json" <<EOF
{
  "relay_url": "${RELAY_URL}",
  "public_relay": "${RELAY_URL}"
}
EOF
  chmod 600 "${HOME}/.runeverything/config.json"
fi

install_launchd() {
  local plist="${HOME}/Library/LaunchAgents/com.runeverything.agent.plist"
  mkdir -p "${HOME}/Library/LaunchAgents"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.runeverything.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>${TARGET}</string>
    <string>run</string>
    <string>-no-qr</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>RE_RELAY</key>
    <string>${RELAY_URL}</string>
  </dict>
  <key>StandardOutPath</key>
  <string>${HOME}/.runeverything/agent.log</string>
  <key>StandardErrorPath</key>
  <string>${HOME}/.runeverything/agent.err</string>
</dict>
</plist>
EOF
  launchctl unload "$plist" 2>/dev/null || true
  launchctl load "$plist"
  # Prefer staying awake on AC; agent also runs caffeinate itself.
  pmset -g custom 2>/dev/null | head -1 >/dev/null || true
  info "Installed launchd agent: $plist"
  info "Tip: keep Mac plugged in; agent prevents idle sleep while running."
}

install_systemd_user() {
  local unit_dir="${HOME}/.config/systemd/user"
  mkdir -p "$unit_dir"
  cat > "${unit_dir}/runeverything.service" <<EOF
[Unit]
Description=RunEverything Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${TARGET} run -no-qr
Restart=always
RestartSec=3
Environment=RE_RELAY=${RELAY_URL}
# Agent calls systemd-inhibit itself; also restart on failure / network blips.

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  systemctl --user enable --now runeverything.service
  # Survive logout / GUI lock: allow user services without interactive session.
  if command -v loginctl >/dev/null 2>&1; then
    loginctl enable-linger "$(id -un)" 2>/dev/null || info "enable-linger failed (need permission); agent may stop on logout"
  fi
  info "Installed systemd user service runeverything.service"
  info "Tip: keep the machine awake; agent inhibits idle sleep while running."
}

case "$os" in
  darwin) install_launchd ;;
  linux)
    if command -v systemctl >/dev/null 2>&1; then
      install_systemd_user || info "systemd user install failed; run ${TARGET} manually"
    else
      info "no systemd; start manually: ${TARGET} run"
    fi
    ;;
esac

# Ensure PATH hint
case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *) info "Add to PATH: export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
esac

info "Installed ${TARGET}"
info "Show pairing QR: ${PREFIX_NAME} pair"
info "Status: ${PREFIX_NAME} status"

# Print QR now (foreground, once)
if [[ -t 1 ]]; then
  "${TARGET}" pair || true
fi
