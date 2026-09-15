#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${RE_VERSION:-0.1.0}}"
OUT="${ROOT}/dist/v${VERSION}"
MANIFEST="${ROOT}/packaging/scoop/bucket/runeverything.json"

[[ -f "${OUT}/runeverything_windows_amd64.zip" ]] || { echo "missing dist; run build-release.sh first" >&2; exit 1; }

python3 - "$MANIFEST" "$VERSION" "$OUT" <<'PY'
import hashlib, json, pathlib, sys
path, version, out = pathlib.Path(sys.argv[1]), sys.argv[2], pathlib.Path(sys.argv[3])

def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()

data = json.loads(path.read_text())
data["version"] = version
data["architecture"]["64bit"]["url"] = f"https://github.com/foqerhk/runeverything/releases/download/v{version}/runeverything_windows_amd64.zip"
data["architecture"]["64bit"]["hash"] = sha(out / "runeverything_windows_amd64.zip")
data["architecture"]["arm64"]["url"] = f"https://github.com/foqerhk/runeverything/releases/download/v{version}/runeverything_windows_arm64.zip"
data["architecture"]["arm64"]["hash"] = sha(out / "runeverything_windows_arm64.zip")
path.write_text(json.dumps(data, indent=2) + "\n")
print(f"updated {path}")
PY
