#!/usr/bin/env bash
# Exercise Mac inject paths matching KoKo「更多 → 手势说明」mappings.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/go/bin:${PATH}"
export GOTOOLCHAIN=auto
cd "$ROOT"
echo "using $(go version)"
CGO_ENABLED=1 go run ./cmd/gesture-selftest
