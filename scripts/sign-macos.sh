#!/usr/bin/env bash
# Sign (and optionally notarize) the macOS agent binary for stable TCC identity.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${1:-}"
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
  echo "usage: $0 /path/to/runeverything" >&2
  exit 2
fi

# Prefer personal Developer ID (stable Team ID across rebuilds). Override with RE_SIGN_IDENTITY.
IDENTITY="${RE_SIGN_IDENTITY:-Developer ID Application: wei liu (U5SLTWD6AH)}"
IDENTIFIER="${RE_BUNDLE_ID:-com.foqerhk.runeverything}"
ENTS="$ROOT/packaging/macos/runeverything.entitlements"

# Optional App Store Connect API (KoKo .env) for notarization.
KOKO_ENV="${RE_ASC_ENV:-$HOME/Downloads/KoKo/.env}"
if [[ -f "$KOKO_ENV" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$KOKO_ENV"; set +a
fi

echo "==> codesign $BIN"
echo "    identity: $IDENTITY"
echo "    identifier: $IDENTIFIER"

codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  --identifier "$IDENTIFIER" \
  --entitlements "$ENTS" \
  "$BIN"

codesign --verify --verbose=2 "$BIN"
echo "==> signed OK"

# Notarization needs network + matching team for the Developer ID cert.
if [[ "${RE_NOTARIZE:-1}" == "1" && -n "${ASC_KEY_ID:-}" && -n "${ASC_ISSUER_ID:-}" && -n "${ASC_PRIVATE_KEY_PATH:-}" ]]; then
  if [[ ! -f "$ASC_PRIVATE_KEY_PATH" ]]; then
    echo "==> skip notarize: ASC key file missing: $ASC_PRIVATE_KEY_PATH" >&2
  else
    ZIP="$(mktemp -t re-sign).zip"
    ditto -c -k --keepParent "$BIN" "$ZIP"
    echo "==> notarize (notarytool)"
    if xcrun notarytool submit "$ZIP" \
      --key "$ASC_PRIVATE_KEY_PATH" \
      --key-id "$ASC_KEY_ID" \
      --issuer "$ASC_ISSUER_ID" \
      --wait; then
      # Stapling only applies to bundles/disk images; bare CLI is fine unsigned-staple.
      echo "==> notarize OK"
    else
      echo "==> notarize failed (binary is still signed; TCC Team ID is stable). Check ASC team matches Developer ID team U5SLTWD6AH." >&2
    fi
    rm -f "$ZIP"
  fi
else
  echo "==> skip notarize (set RE_NOTARIZE=1 and ASC_* in $KOKO_ENV)"
fi
