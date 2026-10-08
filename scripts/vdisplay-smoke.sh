#!/usr/bin/env bash
# Smoke-test re-vdisplay + Agent monitor enumeration (Darwin).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/go/bin:${PATH}"
export GOTOOLCHAIN=auto

HELPER="${RE_VDISPLAY_BIN:-/tmp/re-vdisplay}"
"$ROOT/scripts/build-re-vdisplay.sh" "$HELPER"

echo "==> helper ping"
# ping then quit so the helper exits cleanly
printf '%s\n' '{"cmd":"ping"}' '{"cmd":"quit"}' | "$HELPER" | head -1

export RE_VDISPLAY_BIN="$HELPER"
cd "$ROOT"
echo "==> go run ./cmd/vdisplaysmoke ${1:-8k}"
CGO_ENABLED=1 go run ./cmd/vdisplaysmoke "${1:-8k}"
echo "==> smoke OK"
