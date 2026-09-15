# RunEverything one-line installer (Windows PowerShell)
# Usage: irm https://.../install.ps1 | iex
# Or:   .\scripts\install.ps1

$ErrorActionPreference = "Stop"

$Repo = if ($env:RE_REPO) { $env:RE_REPO } else { "foqerhk/runeverything" }
$Version = if ($env:RE_VERSION) { $env:RE_VERSION } else { "latest" }
$InstallDir = if ($env:RE_INSTALL_DIR) { $env:RE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "RunEverything\bin" }
$RelayUrl = if ($env:RE_RELAY) { $env:RE_RELAY } else { "ws://127.0.0.1:8787/ws" }

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Target = Join-Path $InstallDir "runeverything.exe"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$LocalBin = Join-Path $RepoRoot "bin\runeverything.exe"

if (Test-Path $LocalBin) {
  Write-Host "==> Installing local binary $LocalBin"
  Copy-Item $LocalBin $Target -Force
} elseif (Get-Command go -ErrorAction SilentlyContinue) {
  if (Test-Path (Join-Path $RepoRoot "go.mod")) {
    Write-Host "==> Building from source"
    Push-Location $RepoRoot
    go build -o $Target ./cmd/agent
    Pop-Location
  }
} else {
  $Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
  } else { "amd64" }
  if ($Version -eq "latest") {
    $Url = "https://github.com/$Repo/releases/latest/download/runeverything_windows_$Arch.exe"
  } else {
    $Url = "https://github.com/$Repo/releases/download/$Version/runeverything_windows_$Arch.exe"
  }
  Write-Host "==> Downloading $Url"
  Invoke-WebRequest -Uri $Url -OutFile $Target
}

$HomeDir = Join-Path $env:USERPROFILE ".runeverything"
New-Item -ItemType Directory -Force -Path $HomeDir | Out-Null
$ConfigPath = Join-Path $HomeDir "config.json"
if (-not (Test-Path $ConfigPath)) {
  @{
    relay_url = $RelayUrl
    public_relay = $RelayUrl
  } | ConvertTo-Json | Set-Content -Path $ConfigPath -Encoding UTF8
}

# Scheduled task at logon
$TaskName = "RunEverythingAgent"
$Action = New-ScheduledTaskAction -Execute $Target -Argument "run -no-qr"
$Trigger = New-ScheduledTaskTrigger -AtLogOn
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
try {
  Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
  Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Settings $Settings -Description "RunEverything Agent" | Out-Null
  Start-ScheduledTask -TaskName $TaskName
  Write-Host "==> Registered scheduled task $TaskName"
} catch {
  Write-Host "==> Could not register scheduled task: $_"
  Write-Host "    Start manually: $Target run"
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
  Write-Host "==> Added $InstallDir to user PATH (restart shell)"
}

Write-Host "==> Installed $Target"
Write-Host "==> Pair: runeverything pair"
& $Target pair
