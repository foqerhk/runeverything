#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
NUSPEC="${ROOT}/packaging/chocolatey/runeverything.nuspec"
INSTALL_PS1="${ROOT}/packaging/chocolatey/tools/chocolateyInstall.ps1"

[[ -f "${OUT}/runeverything_windows_amd64.zip" ]] || { echo "missing dist; build-release first" >&2; exit 1; }

python3 - "$NUSPEC" "$INSTALL_PS1" "$VERSION" "$OUT" <<'PY'
import hashlib, pathlib, re, sys, zipfile, tempfile, shutil

nuspec, install_ps1, version, out = map(pathlib.Path, sys.argv[1:5])

def sha256(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()

amd = sha256(out / "runeverything_windows_amd64.zip")
arm = sha256(out / "runeverything_windows_arm64.zip")

ns = nuspec.read_text()
ns = re.sub(r"<version>[^<]+</version>", f"<version>{version}</version>", ns, count=1)
nuspec.write_text(ns)

ps = install_ps1.read_text()
ps = re.sub(r"(?m)^\$version = '.*'", f"$version = '{version}'", ps, count=1)
ps = re.sub(r"(?m)^\$checksumAmd = '.*'", f"$checksumAmd = '{amd}'", ps, count=1)
ps = re.sub(r"(?m)^\$checksumArm = '.*'", f"$checksumArm = '{arm}'", ps, count=1)
install_ps1.write_text(ps)

pack = pathlib.Path(tempfile.mkdtemp())
(pack / "tools").mkdir()
shutil.copy(nuspec, pack / "runeverything.nuspec")
for f in (install_ps1.parent).glob("*.ps1"):
    shutil.copy(f, pack / "tools" / f.name)
nupkg = out / f"runeverything.{version}.nupkg"
with zipfile.ZipFile(nupkg, "w", zipfile.ZIP_DEFLATED) as z:
    for p in pack.rglob("*"):
        if p.is_file():
            z.write(p, p.relative_to(pack).as_posix())
shutil.rmtree(pack)
print(f"updated {install_ps1}")
print(f"  amd64={amd}")
print(f"  arm64={arm}")
print(f"wrote {nupkg}")
PY
