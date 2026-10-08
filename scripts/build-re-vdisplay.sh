#!/usr/bin/env bash
# Build the macOS re-vdisplay helper (CGVirtualDisplay retain process).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/bin/re-vdisplay}"
mkdir -p "$(dirname "$OUT")"
echo "==> clang re-vdisplay -> $OUT"
xcrun clang -fobjc-arc -O2 \
  -framework Foundation -framework CoreGraphics \
  -o "$OUT" \
  "$ROOT/cmd/re-vdisplay/main.m"
echo "==> OK $(ls -la "$OUT")"
