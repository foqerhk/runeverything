# Scoop bucket (staging)

Push this directory to a GitHub repo named **`scoop-runeverything`** (or keep under `foqerhk/scoop-bucket`).

Users:

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything/runeverything
```

Manifest: [`bucket/runeverything.json`](bucket/runeverything.json)

After each release:

```bash
./scripts/sync-scoop-manifest.sh 0.1.0
# commit + push this bucket repo
```
