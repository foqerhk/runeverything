#!/usr/bin/env bash
# Download bundled AkVirtualCamera installers into packaging/akvirtualcamera/.
# Source tree lives in third_party/akvirtualcamera (GPL-3.0).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DST="$ROOT/packaging/akvirtualcamera"
mkdir -p "$DST"
curl -fsSL -L -o "$DST/akvirtualcamera-mac-arm64.pkg" \
  "https://github.com/webcamoid/akvirtualcamera/releases/download/9.3.3/akvirtualcamera-mac-9.3.3-arm64.pkg"
curl -fsSL -L -o "$DST/akvirtualcamera-mac-x64.pkg" \
  "https://github.com/webcamoid/akvirtualcamera/releases/download/9.2.0/akvirtualcamera-mac-9.2.0-x64.pkg"
curl -fsSL -L -o "$DST/akvirtualcamera-windows.exe" \
  "https://github.com/webcamoid/akvirtualcamera/releases/download/9.4.1/akvirtualcamera-windows-9.4.1.exe"
cat > "$DST/VERSION" <<'EOF'
mac_arm64=akvirtualcamera-mac-9.3.3-arm64.pkg
mac_x64=akvirtualcamera-mac-9.2.0-x64.pkg
windows=akvirtualcamera-windows-9.4.1.exe
source=third_party/akvirtualcamera (Webcamoid AkVirtualCamera, GPL-3.0)
EOF
ls -la "$DST"
echo "==> OK"
