# Install RunEverything via winget using manifests from this repo (no wait for winget-pkgs merge).
# Usage:
#   irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-winget.ps1 | iex
# Or:
#   .\scripts\install-winget.ps1

$ErrorActionPreference = "Stop"

if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
  Write-Error "winget not found. Use install.ps1 or Scoop instead: irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex"
}

$Repo = if ($env:RE_REPO) { $env:RE_REPO } else { "foqerhk/runeverything" }
$Version = if ($env:RE_VERSION) { $env:RE_VERSION } else { "0.1.0" }
$Ver = $Version.TrimStart('v')

$Base = "https://raw.githubusercontent.com/$Repo/main/packaging/winget/Foqerhk.RunEverything/$Ver"
$Dir = Join-Path $env:TEMP "runeverything-winget-$Ver"
New-Item -ItemType Directory -Force -Path $Dir | Out-Null

$files = @(
  "Foqerhk.RunEverything.yaml",
  "Foqerhk.RunEverything.installer.yaml",
  "Foqerhk.RunEverything.locale.en-US.yaml"
)

Write-Host "==> Downloading winget manifests ($Ver)"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
foreach ($f in $files) {
  $url = "$Base/$f"
  Write-Host "    $url"
  Invoke-WebRequest -Uri $url -OutFile (Join-Path $Dir $f) -UseBasicParsing
}

Write-Host "==> winget install --manifest $Dir"
winget install --manifest $Dir --accept-package-agreements --accept-source-agreements

Write-Host ""
Write-Host "==> Done. Pair with: runeverything pair"
Write-Host "    Set relay: `$env:RE_RELAY = 'wss://your-relay.example/re2'"
