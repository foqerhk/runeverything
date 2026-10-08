#!/usr/bin/env bash
# Build the macOS re-phonecam helper (phone → virtual webcam + preview).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/bin/re-phonecam}"
mkdir -p "$(dirname "$OUT")"
echo "==> clang re-phonecam -> $OUT"
xcrun clang -fobjc-arc -O2 \
  -framework Foundation -framework AppKit -framework CoreGraphics -framework ImageIO \
  -framework VideoToolbox -framework CoreMedia -framework CoreVideo \
  -o "$OUT" \
  "$ROOT/cmd/re-phonecam/main.m"
echo "==> OK $(ls -la "$OUT")"
