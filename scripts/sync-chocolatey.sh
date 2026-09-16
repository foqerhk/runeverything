#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
NUSPEC="${ROOT}/packaging/chocolatey/runeverything.nuspec"
INSTALL_PS1="${ROOT}/packaging/chocolatey/tools/chocolateyInstall.ps1"

[[ -f "${OUT}/runeverything_windows_amd64.zip" ]] || { echo "missing dist; build-release first" >&2; exit 1; }

sha_of() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

AMD=$(sha_of "${OUT}/runeverything_windows_amd64.zip")
ARM=$(sha_of "${OUT}/runeverything_windows_arm64.zip")

perl -i -pe "s#<version>[^<]+</version>#<version>${VERSION}</version>#" "$NUSPEC"
perl -i -pe "s/^\\\$version = '.*'/\$version = '${VERSION}'/" "$INSTALL_PS1"
perl -i -pe "s/^\\\$checksumAmd = '.*'/\$checksumAmd = '${AMD}'/" "$INSTALL_PS1"
perl -i -pe "s/^\\\$checksumArm = '.*'/\$checksumArm = '${ARM}'/" "$INSTALL_PS1"

PACK_DIR="$(mktemp -d)"
mkdir -p "${PACK_DIR}/tools"
cp "$NUSPEC" "${PACK_DIR}/runeverything.nuspec"
cp "${ROOT}/packaging/chocolatey/tools/"*.ps1 "${PACK_DIR}/tools/"
python3 - "$PACK_DIR" "${OUT}/runeverything.${VERSION}.nupkg" <<'PY'
import zipfile, pathlib, sys
root = pathlib.Path(sys.argv[1])
out = pathlib.Path(sys.argv[2])
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for p in root.rglob("*"):
        if p.is_file():
            z.write(p, p.relative_to(root).as_posix())
print("wrote", out)
PY
rm -rf "$PACK_DIR"
echo "updated chocolatey package for v${VERSION}"
