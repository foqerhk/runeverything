#!/usr/bin/env bash
# Build + sign AkVirtualCamera (DAL plugin + Camera Extension) into
# packaging/akvirtualcamera/macos-payload for local “去设置” install.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/third_party/akvirtualcamera"
BUILD="${RE_AKVCAM_BUILD:-/tmp/build-AkVirtualCamera-RE}"
STAGE="${RE_AKVCAM_STAGE:-/tmp/AkVirtualCamera-RE-install}"
OUT="$ROOT/packaging/akvirtualcamera/macos-payload"
IDENTITY="${RE_SIGN_IDENTITY:-Developer ID Application: wei liu (U5SLTWD6AH)}"
TEAM="${RE_DEVELOPMENT_TEAM:-U5SLTWD6AH}"
ENTS_APP="$ROOT/packaging/akvirtualcamera/AkVirtualCameraCX.entitlements"
ENTS_EXT="$ROOT/packaging/akvirtualcamera/AkVirtualCameraCX.Extension.entitlements"
PROFILE_APP="${RE_VCAMCX_PROFILE_APP:-}"
PROFILE_EXT="${RE_VCAMCX_PROFILE_EXT:-}"

export DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode-27.1-beta.app/Contents/Developer}"
export PATH="$DEVELOPER_DIR/usr/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"

if [[ ! -f "$SRC/CMakeLists.txt" ]]; then
  echo "missing vendored source: $SRC" >&2
  exit 2
fi

# APP_IDENTIFIER + "CX" / "CX.Extension" → com.foqerhk.runeverything.vcamCX[.Extension]
# Keep ORGANIZATION_IDENTIFIER / CMIO assistant prefs on webcamoid so AkVCamManager
# devices stay visible to the extension (io.github.webcamoid.AkVirtualCamera.AkVCamAssistant).
APP_ID="${RE_VCAMCX_APP_IDENTIFIER:-com.foqerhk.runeverything.vcam}"

echo "==> cmake configure (team=$TEAM app_id=$APP_ID)"
cmake -S "$SRC" -B "$BUILD" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_INSTALL_PREFIX="$STAGE" \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DCMAKE_XCODE_ATTRIBUTE_DEVELOPMENT_TEAM="$TEAM" \
  -DAPP_IDENTIFIER="$APP_ID"

echo "==> build"
cmake --build "$BUILD" --parallel "$(sysctl -n hw.ncpu)"

echo "==> install stage -> $STAGE"
rm -rf "$STAGE"
cmake --install "$BUILD"

PLUGIN="$STAGE/AkVirtualCamera.plugin"
CX="$STAGE/AkVirtualCameraCX.app"
# Prefer foqerhk bundle id (patched); fall back to upstream webcamoid id.
EXT="$CX/Contents/Library/SystemExtensions/com.foqerhk.runeverything.vcamCX.Extension.systemextension"
if [[ ! -d "$EXT" ]]; then
  EXT="$CX/Contents/Library/SystemExtensions/io.github.webcamoid.AkVirtualCameraCX.Extension.systemextension"
fi

embed_profile() {
  local bundle="$1" profile="$2"
  if [[ -n "$profile" && -f "$profile" ]]; then
    echo "    embed profile -> $bundle"
    cp "$profile" "$bundle/Contents/embedded.provisionprofile"
  fi
}

# Embed Webcamoid-inspired app icon (packaging/akvirtualcamera/icon).
ICON_ICNS="$ROOT/packaging/akvirtualcamera/icon/AkVirtualCameraCX.icns"
if [[ -f "$ICON_ICNS" ]]; then
  echo "==> embed AppIcon.icns"
  mkdir -p "$CX/Contents/Resources"
  cp "$ICON_ICNS" "$CX/Contents/Resources/AppIcon.icns"
  /usr/libexec/PlistBuddy -c 'Delete :CFBundleIconFile' "$CX/Contents/Info.plist" 2>/dev/null || true
  /usr/libexec/PlistBuddy -c 'Add :CFBundleIconFile string AppIcon' "$CX/Contents/Info.plist" 2>/dev/null \
    || /usr/libexec/PlistBuddy -c 'Set :CFBundleIconFile AppIcon' "$CX/Contents/Info.plist"
  /usr/libexec/PlistBuddy -c 'Delete :CFBundleIconName' "$CX/Contents/Info.plist" 2>/dev/null || true
  /usr/libexec/PlistBuddy -c 'Add :CFBundleIconName string AppIcon' "$CX/Contents/Info.plist" 2>/dev/null \
    || /usr/libexec/PlistBuddy -c 'Set :CFBundleIconName AppIcon' "$CX/Contents/Info.plist"
else
  echo "==> warn: missing $ICON_ICNS (CX will use default icon)" >&2
fi

echo "==> codesign (identity=$IDENTITY)"
codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  "$PLUGIN/Contents/Frameworks/libvcam_capi.dylib"
codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  "$PLUGIN/Contents/Resources/AkVCamAssistant"
codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  "$PLUGIN/Contents/Resources/AkVCamManager"
codesign --force --options runtime --timestamp \
  --sign "$IDENTITY" \
  "$PLUGIN/Contents/MacOS/AkVirtualCamera"
codesign --force --deep --options runtime --timestamp \
  --sign "$IDENTITY" \
  "$PLUGIN"

embed_profile "$EXT" "${PROFILE_EXT}"
codesign --force --options runtime --timestamp \
  --entitlements "$ENTS_EXT" \
  --sign "$IDENTITY" \
  "$EXT"
embed_profile "$CX" "${PROFILE_APP}"
codesign --force --options runtime --timestamp \
  --entitlements "$ENTS_APP" \
  --sign "$IDENTITY" \
  "$CX/Contents/MacOS/AkVirtualCameraCX"
codesign --force --deep --options runtime --timestamp \
  --entitlements "$ENTS_APP" \
  --sign "$IDENTITY" \
  "$CX"

echo "==> stage payload -> $OUT"
rm -rf "$OUT"
mkdir -p "$OUT"
cp -R "$PLUGIN" "$OUT/"
cp -R "$CX" "$OUT/"
cp "$ROOT/scripts/install-akvirtualcamera-macos.sh" "$OUT/install.sh"
chmod +x "$OUT/install.sh"
cat > "$OUT/README.txt" <<EOF
RunEverything bundled AkVirtualCamera (Camera Extension + DAL).
Install via Agent permissions panel, or:
  open install.sh  (admin password)
Then open AkVirtualCameraCX.app → Install extension → allow in System Settings.
In Zoom/Meet/browser, select camera “KoKo Phone Camera”.
EOF

echo "==> OK"
ls -la "$OUT"
