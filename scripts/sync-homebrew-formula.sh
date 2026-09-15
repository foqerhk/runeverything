#!/usr/bin/env bash
# Refresh packaging/homebrew-tap/Formula/runeverything.rb sha256 from dist/ after build-release.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
FORMULA="${ROOT}/packaging/homebrew-tap/Formula/runeverything.rb"

[[ -d "$OUT" ]] || { echo "missing ${OUT}; run scripts/build-release.sh ${VERSION} first" >&2; exit 1; }
[[ -f "$FORMULA" ]] || { echo "missing formula ${FORMULA}" >&2; exit 1; }

python3 - "$FORMULA" "$VERSION" "$OUT" <<'PY'
import hashlib, pathlib, re, sys

path = pathlib.Path(sys.argv[1])
version = sys.argv[2]
out = pathlib.Path(sys.argv[3])

def sha256(p: pathlib.Path) -> str:
    h = hashlib.sha256()
    h.update(p.read_bytes())
    return h.hexdigest()

shas = {
    "darwin_arm64": sha256(out / "runeverything_darwin_arm64.tar.gz"),
    "darwin_amd64": sha256(out / "runeverything_darwin_amd64.tar.gz"),
    "linux_arm64": sha256(out / "runeverything_linux_arm64.tar.gz"),
    "linux_amd64": sha256(out / "runeverything_linux_amd64.tar.gz"),
}

text = path.read_text()
text = re.sub(r'version "[^"]*"', f'version "{version}"', text, count=1)

for key, digest in shas.items():
    os_name, arch = key.split("_", 1)
    pat = rf'(runeverything_{os_name}_{arch}\.tar\.gz"\n\s*sha256 ")([^"]+)(")'
    text2, n = re.subn(pat, rf"\g<1>{digest}\g<3>", text, count=1)
    if n != 1:
        raise SystemExit(f"failed to patch sha for {key}")
    text = text2

path.write_text(text)
print(f"updated {path}")
for k, v in shas.items():
    print(f"  {k}={v}")
PY

echo
echo "Push packaging/homebrew-tap to GitHub as: foqerhk/homebrew-tap"
echo "Users install with:"
echo "  brew install foqerhk/tap/runeverything"
