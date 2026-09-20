#!/usr/bin/env bash
# First-run permission & autostart hints for home users (non-account / public-good model).
set -euo pipefail
OS="$(uname -s)"
echo "==> RunEverything setup checklist (no account required)"
echo

case "$OS" in
  Darwin)
    echo "macOS — grant these once so remote desktop works:"
    echo "  1. System Settings → Privacy & Security → Screen Recording → enable Terminal / runeverything"
    echo "  2. Privacy & Security → Accessibility → enable for input injection"
    echo "  3. Keep Agent awake: launchd install via scripts/install.sh (default)"
    echo "  Optional: RE_PAIR_CONFIRM=1 to approve each new scan on the Mac"
    ;;
  Linux)
    echo "Linux — ensure a graphical session is logged in (X11/Wayland)."
    echo "  DISPLAY is usually :0 locally, or :10/:11 under XRDP."
    echo "  Install: libx11 / libxtst / ffmpeg; optional xclip for clipboard."
    echo "  Autostart: scripts/install.sh installs a systemd --user unit."
    echo "  Measure relay capacity before sharing: scripts/probe-bandwidth.sh"
    ;;
  MINGW*|MSYS*|CYGWIN*|Windows*)
    echo "Windows — run Agent as the logged-in desktop user (not Session 0)."
    echo "  UAC / secure desktop may block injection until approved on the PC."
    echo "  Autostart: scripts/install.ps1 scheduled task."
    ;;
esac
echo
echo "Volunteer relays: run scripts/probe-bandwidth.sh then start relay so"
echo "RE_MAX_SESSIONS matches your uplink (busy+latency selection on home agents)."
echo "Done. Pair with KoKo by scanning the QR (v=3). No signup."
