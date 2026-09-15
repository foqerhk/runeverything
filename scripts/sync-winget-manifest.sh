#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
DIR="${ROOT}/packaging/winget/Foqerhk.RunEverything/${VERSION}"

[[ -d "$DIR" ]] || { echo "missing ${DIR}" >&2; exit 1; }
[[ -f "${OUT}/runeverything_windows_amd64.zip" ]] || { echo "missing dist" >&2; exit 1; }

python3 - "$DIR" "$VERSION" "$OUT" <<'PY'
import hashlib, pathlib, re, sys
d, version, out = pathlib.Path(sys.argv[1]), sys.argv[2], pathlib.Path(sys.argv[3])

def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest().upper()

amd = sha(out / "runeverything_windows_amd64.zip")
arm = sha(out / "runeverything_windows_arm64.zip")

inst = d / "Foqerhk.RunEverything.installer.yaml"
text = inst.read_text()
text = re.sub(
    r'(runeverything_windows_amd64\.zip\n\s*InstallerSha256:\s*)(\S+)',
    rf'\g<1>{amd}', text, count=1)
text = re.sub(
    r'(runeverything_windows_arm64\.zip\n\s*InstallerSha256:\s*)(\S+)',
    rf'\g<1>{arm}', text, count=1)
# also replace placeholders
text = text.replace("REPLACE_WINDOWS_AMD64_SHA256", amd).replace("REPLACE_WINDOWS_ARM64_SHA256", arm)
inst.write_text(text)
print(f"updated {inst}")
print(f"  amd64={amd}")
print(f"  arm64={arm}")
PY
