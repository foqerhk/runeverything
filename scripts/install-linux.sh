#!/usr/bin/env bash
# Auto-detect Linux distro and install via the matching package manager.
set -euo pipefail

SCRIPT_BASE="${RE_SCRIPT_BASE:-https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts}"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This installer is for Linux. On macOS/Windows use scripts/install.sh or install.ps1." >&2
  exit 1
fi

id_like=""
id_name=""
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  id_name="${ID:-}"
  id_like="${ID_LIKE:-}"
fi

run_remote() {
  local name="$1"
  echo "==> Using ${name}"
  curl -fsSL "${SCRIPT_BASE}/${name}" | bash
}

case "${id_name}" in
  debian|ubuntu|linuxmint|pop|raspbian|elementary)
    curl -fsSL "${SCRIPT_BASE}/install-apt.sh" | sudo bash
    ;;
  fedora|rhel|centos|rocky|almalinux|ol|amzn)
    curl -fsSL "${SCRIPT_BASE}/install-rpm.sh" | sudo bash
    ;;
  opensuse*|sles)
    curl -fsSL "${SCRIPT_BASE}/install-rpm.sh" | sudo bash
    ;;
  arch|manjaro|endeavouros|garuda)
    curl -fsSL "${SCRIPT_BASE}/install-arch.sh" | sudo bash
    ;;
  alpine)
    curl -fsSL "${SCRIPT_BASE}/install-apk.sh" | sudo bash
    ;;
  *)
    # Fallbacks by available tools / ID_LIKE
    if echo "${id_like}" | grep -Eq 'debian|ubuntu'; then
      curl -fsSL "${SCRIPT_BASE}/install-apt.sh" | sudo bash
    elif echo "${id_like}" | grep -Eq 'rhel|fedora|centos|suse'; then
      curl -fsSL "${SCRIPT_BASE}/install-rpm.sh" | sudo bash
    elif echo "${id_like}" | grep -Eq 'arch'; then
      curl -fsSL "${SCRIPT_BASE}/install-arch.sh" | sudo bash
    elif command -v apt-get >/dev/null 2>&1; then
      curl -fsSL "${SCRIPT_BASE}/install-apt.sh" | sudo bash
    elif command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1 || command -v zypper >/dev/null 2>&1; then
      curl -fsSL "${SCRIPT_BASE}/install-rpm.sh" | sudo bash
    elif command -v pacman >/dev/null 2>&1; then
      curl -fsSL "${SCRIPT_BASE}/install-arch.sh" | sudo bash
    elif command -v apk >/dev/null 2>&1; then
      curl -fsSL "${SCRIPT_BASE}/install-apk.sh" | sudo bash
    else
      echo "==> Unknown distro (${id_name:-?}); falling back to generic install.sh"
      curl -fsSL "${SCRIPT_BASE}/install.sh" | bash
    fi
    ;;
esac
