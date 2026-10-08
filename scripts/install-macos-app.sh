#!/usr/bin/env bash
# Assemble /Applications/RunEverything.app from a signed agent binary.
# Optionally installs Contents/MacOS/re-vdisplay and re-phonecam helpers.
#
# Local 联调 (default): RE_NOTARIZE=0 — codesign with personal Developer ID + entitlements.
# Distribution: RE_NOTARIZE=1 — also notarize .app (skips unsigned akvirtualcamera pkgs/payload).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${1:-}"
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
  echo "usage: $0 /path/to/runeverything [ /path/to/re-vdisplay ]" >&2
  exit 2
fi
VDISPLAY_BIN="${2:-}"
if [[ -z "$VDISPLAY_BIN" ]]; then
  if [[ -x /tmp/re-vdisplay ]]; then
    VDISPLAY_BIN=/tmp/re-vdisplay
  elif [[ -x "$ROOT/bin/re-vdisplay" ]]; then
    VDISPLAY_BIN="$ROOT/bin/re-vdisplay"
  fi
fi

APP="${RE_APP_PATH:-/Applications/RunEverything.app}"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RES="$CONTENTS/Resources"
mkdir -p "$MACOS" "$RES"
cp "$ROOT/packaging/macos/Info.plist" "$CONTENTS/Info.plist"
cp "$ROOT/packaging/macos/AppIcon.icns" "$RES/AppIcon.icns"
# Never leave stray bak binaries inside the bundle (breaks deep codesign).
find "$MACOS" -maxdepth 1 -type f \( -name '*.bak*' -o -name 'RunEverything.bak-*' \) -delete 2>/dev/null || true
rm -f "$MACOS/RunEverything" "$MACOS/runeverything"
cp "$BIN" "$MACOS/RunEverything"
chmod +x "$MACOS/RunEverything"

if [[ -n "$VDISPLAY_BIN" && -f "$VDISPLAY_BIN" ]]; then
  cp "$VDISPLAY_BIN" "$MACOS/re-vdisplay"
  chmod +x "$MACOS/re-vdisplay"
  echo "==> bundled re-vdisplay"
elif [[ ! -x "$MACOS/re-vdisplay" ]]; then
  echo "==> building re-vdisplay"
  "$ROOT/scripts/build-re-vdisplay.sh" "$MACOS/re-vdisplay"
fi

PHONECAM_BIN=""
if [[ -x /tmp/re-phonecam ]]; then
  PHONECAM_BIN=/tmp/re-phonecam
elif [[ -x "$ROOT/bin/re-phonecam" ]]; then
  PHONECAM_BIN="$ROOT/bin/re-phonecam"
fi
if [[ -n "$PHONECAM_BIN" ]]; then
  cp "$PHONECAM_BIN" "$MACOS/re-phonecam"
  chmod +x "$MACOS/re-phonecam"
  echo "==> bundled re-phonecam"
elif [[ ! -x "$MACOS/re-phonecam" ]]; then
  echo "==> building re-phonecam"
  "$ROOT/scripts/build-re-phonecam.sh" "$MACOS/re-phonecam"
fi

# Optional “用手机当摄像头” — Camera Extension payload (+ legacy pkgs if present).
# Notarized builds cannot include unsigned nested pkgs / adhoc plugin — strip those when RE_NOTARIZE=1.
AKVCAM_SRC="$ROOT/packaging/akvirtualcamera"
AKVCAM_DST="$RES/akvirtualcamera"
if [[ -d "$AKVCAM_SRC" ]]; then
  mkdir -p "$AKVCAM_DST"
  if [[ -d "$AKVCAM_SRC/macos-payload" ]]; then
    rm -rf "$AKVCAM_DST/macos-payload"
    cp -R "$AKVCAM_SRC/macos-payload" "$AKVCAM_DST/macos-payload"
    echo "==> bundled akvirtualcamera macos-payload (Camera Extension)"
  fi
  for f in VERSION README.md \
           AkVirtualCameraCX.entitlements AkVirtualCameraCX.Extension.entitlements; do
    if [[ -f "$AKVCAM_SRC/$f" ]]; then
      cp "$AKVCAM_SRC/$f" "$AKVCAM_DST/$f"
    fi
  done
  if [[ "${RE_NOTARIZE:-0}" == "1" ]]; then
    echo "==> notarize mode: omit legacy akvirtualcamera-mac-*.pkg (unsigned nested code)"
    rm -f "$AKVCAM_DST"/akvirtualcamera-mac-*.pkg 2>/dev/null || true
    # Adhoc/Development-signed payload also fails notary — drop for notarized shipping.
    rm -rf "$AKVCAM_DST/macos-payload"
    echo "==> notarize mode: omit macos-payload (adhoc / Apple Development nested code)"
  else
    for f in akvirtualcamera-mac-arm64.pkg akvirtualcamera-mac-x64.pkg; do
      if [[ -f "$AKVCAM_SRC/$f" ]]; then
        cp "$AKVCAM_SRC/$f" "$AKVCAM_DST/$f"
      fi
    done
  fi
fi

# Sign binaries then the whole app (preferred for TCC icon / identity).
IDENTITY="${RE_SIGN_IDENTITY:-Developer ID Application: wei liu (U5SLTWD6AH)}"
ENTS="$ROOT/packaging/macos/runeverything.entitlements"
if [[ ! -x "$ROOT/scripts/sign-macos.sh" ]]; then
  echo "missing $ROOT/scripts/sign-macos.sh" >&2
  exit 1
fi

# Binary codesign only here; optional app notarize below.
RE_NOTARIZE=0 "$ROOT/scripts/sign-macos.sh" "$MACOS/RunEverything"
if [[ -x "$MACOS/re-vdisplay" ]]; then
  codesign --force --options runtime --timestamp \
    --sign "$IDENTITY" \
    --identifier "com.foqerhk.runeverything.vdisplay" \
    --entitlements "$ENTS" \
    "$MACOS/re-vdisplay"
fi
if [[ -x "$MACOS/re-phonecam" ]]; then
  codesign --force --options runtime --timestamp \
    --sign "$IDENTITY" \
    --identifier "com.foqerhk.runeverything.phonecam" \
    --entitlements "$ENTS" \
    "$MACOS/re-phonecam"
fi
# Sign inside-out exactly once. `--deep` during signing recursively re-signs the
# main executable after sign-macos.sh and can replace the code requirement TCC
# recorded for ScreenCapture/Accessibility. Keep `--deep` for verification only.
codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  --identifier "com.foqerhk.runeverything" \
  --entitlements "$ENTS" \
  "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

if [[ "${RE_NOTARIZE:-0}" == "1" ]]; then
  KOKO_ENV="${RE_ASC_ENV:-$HOME/Downloads/KoKo/.env}"
  if [[ -f "$KOKO_ENV" ]]; then
    # shellcheck disable=SC1090
    set -a; source "$KOKO_ENV"; set +a
  fi
  if [[ -n "${ASC_KEY_ID:-}" && -n "${ASC_ISSUER_ID:-}" && -n "${ASC_PRIVATE_KEY_PATH:-}" && -f "${ASC_PRIVATE_KEY_PATH}" ]]; then
    ZIP="$(mktemp -t re-app).zip"
    ditto -c -k --keepParent "$APP" "$ZIP"
    echo "==> notarize app (notarytool)"
    SUBMIT_OUT="$(mktemp -t re-notary-out)"
    if ! xcrun notarytool submit "$ZIP" \
      --key "$ASC_PRIVATE_KEY_PATH" \
      --key-id "$ASC_KEY_ID" \
      --issuer "$ASC_ISSUER_ID" \
      --wait 2>&1 | tee "$SUBMIT_OUT"; then
      rm -f "$ZIP" "$SUBMIT_OUT"
      echo "==> notarize submit failed" >&2
      exit 1
    fi
    rm -f "$ZIP"
    if ! grep -q 'status: Accepted' "$SUBMIT_OUT"; then
      echo "==> notarize did not report Accepted" >&2
      cat "$SUBMIT_OUT" >&2
      rm -f "$SUBMIT_OUT"
      exit 1
    fi
    rm -f "$SUBMIT_OUT"
    echo "==> staple $APP"
    xcrun stapler staple "$APP"
    spctl -a -vv "$APP"
  else
    echo "==> skip app notarize: ASC_* missing in $KOKO_ENV" >&2
  fi
fi

# Symlink CLI helper for PATH users.
if [[ -w /usr/local/bin ]] || [[ -w /usr/local/Cellar/runeverything/0.2.3-dev/bin ]]; then
  if [[ -d /usr/local/Cellar/runeverything/0.2.3-dev/bin ]]; then
    cp "$MACOS/RunEverything" /usr/local/Cellar/runeverything/0.2.3-dev/bin/runeverything
  fi
  ln -sfn "$MACOS/RunEverything" /usr/local/bin/runeverything 2>/dev/null || true
fi

echo "==> installed $APP"
echo "    launch: $MACOS/RunEverything tray"
echo "    or:     open -a RunEverything --args tray"
echo "    vdisplay: RE_VDISPLAY=8k $MACOS/RunEverything tray"
