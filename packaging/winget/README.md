# winget manifests (staging)

These files are ready to submit as a PR to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs)
under `manifests/f/Foqerhk/RunEverything/<version>/`.

Until that PR is merged, Windows users can use:

```powershell
# Scoop
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything

# Or one-liner script
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

After each release, refresh SHA256:

```bash
./scripts/sync-winget-manifest.sh 0.1.0
```
