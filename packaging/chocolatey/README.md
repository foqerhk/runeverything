# Chocolatey package (staging)

Build / refresh after a release:

```bash
./scripts/sync-chocolatey.sh 0.1.0
```

Produces `dist/v0.1.0/runeverything.0.1.0.nupkg`.

### Users

**One-liner (no Chocolatey required)** — preferred for most people:

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

**With Chocolatey** (after nupkg is on the Release or a feed):

```powershell
choco install runeverything -y --source "https://github.com/foqerhk/runeverything/releases/download/v0.1.0/"
```

Or install a local nupkg:

```powershell
choco install runeverything -y --source .
```
