#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
PB="${ROOT}/packaging/aur/PKGBUILD"

python3 - "$PB" "$VERSION" "$OUT" <<'PY'
import hashlib, pathlib, re, sys
pb, version, out = pathlib.Path(sys.argv[1]), sys.argv[2], pathlib.Path(sys.argv[3])

def sha(name):
    return hashlib.sha256((out / name).read_bytes()).hexdigest()

text = pb.read_text()
text = re.sub(r'^pkgver=.*$', f'pkgver={version}', text, count=1, flags=re.M)
text = re.sub(r"^sha256sums_x86_64=\('.*'\)$", f"sha256sums_x86_64=('{sha('runeverything_linux_amd64.tar.gz')}')", text, count=1, flags=re.M)
text = re.sub(r"^sha256sums_aarch64=\('.*'\)$", f"sha256sums_aarch64=('{sha('runeverything_linux_arm64.tar.gz')}')", text, count=1, flags=re.M)
pb.write_text(text)
print(f"updated {pb}")
PY
