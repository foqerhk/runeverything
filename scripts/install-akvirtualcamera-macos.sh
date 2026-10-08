#!/usr/bin/env bash
# Privileged install of bundled AkVirtualCamera (plugin + Camera Extension host).
# Intended to run via osascript "with administrator privileges".
set -euo pipefail

PAYLOAD="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_SRC="$PAYLOAD/AkVirtualCamera.plugin"
CX_SRC="$PAYLOAD/AkVirtualCameraCX.app"
INSTALL_ROOT=/Applications/AkVirtualCamera
DAL_LINK=/Library/CoreMediaIO/Plug-Ins/DAL/AkVirtualCamera.plugin
CX_DST=/Applications/AkVirtualCameraCX.app

if [[ ! -d "$PLUGIN_SRC" || ! -d "$CX_SRC" ]]; then
  echo "missing payload next to install.sh" >&2
  exit 2
fi

mkdir -p "$INSTALL_ROOT"
rm -rf "$INSTALL_ROOT/AkVirtualCamera.plugin"
cp -R "$PLUGIN_SRC" "$INSTALL_ROOT/AkVirtualCamera.plugin"
chmod a+x "$INSTALL_ROOT/AkVirtualCamera.plugin/Contents/Resources/AkVCamAssistant" || true
chmod a+x "$INSTALL_ROOT/AkVirtualCamera.plugin/Contents/Resources/AkVCamManager" || true

mkdir -p /Library/CoreMediaIO/Plug-Ins/DAL
rm -rf "$DAL_LINK"
ln -s "$INSTALL_ROOT/AkVirtualCamera.plugin" "$DAL_LINK"

rm -rf "$CX_DST"
cp -R "$CX_SRC" "$CX_DST"

# Launch daemon for AkVCamAssistant (IPC used by Manager / stream).
PLIST=/Library/LaunchDaemons/org.webcamoid.cmio.AkVCam.Assistant.plist
ASSISTANT="$INSTALL_ROOT/AkVirtualCamera.plugin/Contents/Resources/AkVCamAssistant"
cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>org.webcamoid.cmio.AkVCam.Assistant</string>
  <key>ProgramArguments</key>
  <array>
    <string>${ASSISTANT}</string>
    <string>--timeout</string>
    <string>300.0</string>
  </array>
  <key>MachServices</key>
  <dict>
    <key>org.webcamoid.cmio.AkVCam.Assistant</key><true/>
  </dict>
  <key>StandardOutPath</key><string>/tmp/AkVCamAssistant.log</string>
  <key>StandardErrorPath</key><string>/tmp/AkVCamAssistant.log</string>
</dict>
</plist>
EOF
launchctl bootout system/org.webcamoid.cmio.AkVCam.Assistant 2>/dev/null || true
launchctl bootstrap system "$PLIST" 2>/dev/null || launchctl load -w "$PLIST" 2>/dev/null || true
launchctl kickstart -k system/org.webcamoid.cmio.AkVCam.Assistant 2>/dev/null || true

echo "AkVirtualCamera installed. Open AkVirtualCameraCX to activate Camera Extension."
