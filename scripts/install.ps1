# RunEverything Windows installer (PowerShell)
# One-liner:
#   irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
#
# Optional env:
#   $env:RE_RELAY = "wss://your-relay.example/ws"
#   $env:RE_VERSION = "0.1.0"

$ErrorActionPreference = "Stop"

$Repo = if ($env:RE_REPO) { $env:RE_REPO } else { "foqerhk/runeverything" }
$Version = if ($env:RE_VERSION) { $env:RE_VERSION } else { "latest" }
$InstallDir = if ($env:RE_INSTALL_DIR) { $env:RE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "RunEverything\bin" }
$RelayUrl = if ($env:RE_RELAY) { $env:RE_RELAY } else { "ws://127.0.0.1:8787/ws" }

Write-Host "==> RunEverything Windows installer"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Target = Join-Path $InstallDir "runeverything.exe"

$ScriptDir = $null
if ($PSCommandPath) { $ScriptDir = Split-Path -Parent $PSCommandPath }
elseif ($MyInvocation.MyCommand.Path) { $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path }

$installedFrom = $null
if ($ScriptDir) {
  $RepoRoot = Split-Path -Parent $ScriptDir
  $LocalBin = Join-Path $RepoRoot "bin\runeverything.exe"
  if (Test-Path $LocalBin) {
    Write-Host "==> Installing local binary $LocalBin"
    Copy-Item $LocalBin $Target -Force
    $installedFrom = "local"
  }
}

if (-not $installedFrom) {
  $Arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
  if ($Version -eq "latest") {
    $Base = "https://github.com/$Repo/releases/latest/download"
  } else {
    $Ver = $Version
    if (-not $Ver.StartsWith("v")) { $Ver = "v$Ver" }
    $Base = "https://github.com/$Repo/releases/download/$Ver"
  }
  $ZipName = "runeverything_windows_$Arch.zip"
  $ZipUrl = "$Base/$ZipName"
  $Tmp = Join-Path $env:TEMP $ZipName
  Write-Host "==> Downloading $ZipUrl"
  try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $ZipUrl -OutFile $Tmp -UseBasicParsing
    Expand-Archive -Path $Tmp -DestinationPath $InstallDir -Force
    Remove-Item $Tmp -Force -ErrorAction SilentlyContinue
    if (-not (Test-Path $Target)) {
      throw "runeverything.exe not found after unzip"
    }
    $installedFrom = "release"
  } catch {
    $RawUrl = "$Base/runeverything_windows_$Arch.exe"
    Write-Host "==> zip failed ($_); trying $RawUrl"
    Invoke-WebRequest -Uri $RawUrl -OutFile $Target -UseBasicParsing
    $installedFrom = "release-exe"
  }
}

$HomeDir = Join-Path $env:USERPROFILE ".runeverything"
New-Item -ItemType Directory -Force -Path $HomeDir | Out-Null
$ConfigPath = Join-Path $HomeDir "config.json"
if (-not (Test-Path $ConfigPath)) {
  @{
    relay_url    = $RelayUrl
    public_relay = $RelayUrl
  } | ConvertTo-Json | Set-Content -Path $ConfigPath -Encoding UTF8
  Write-Host "==> Wrote $ConfigPath"
} else {
  Write-Host "==> Keep existing $ConfigPath"
}

# PATH (user + current session)
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not $userPath) { $userPath = "" }
if ($userPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
  Write-Host "==> Added to user PATH: $InstallDir"
}
if ($env:Path -notlike "*$InstallDir*") {
  $env:Path = "$InstallDir;$env:Path"
}

# Autostart via scheduled task (runs at logon; agent prevents sleep itself)
$TaskName = "RunEverythingAgent"
try {
  $Action = New-ScheduledTaskAction -Execute $Target -Argument "run -no-qr"
  $Trigger = New-ScheduledTaskTrigger -AtLogOn
  $Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -DontStopOnIdleEnd `
    -StartWhenAvailable `
    -RestartCount 999 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero)
  Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
  Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Settings $Settings -Description "RunEverything Agent (keep PC awake while running)" | Out-Null
  Start-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
  Write-Host "==> Scheduled task: $TaskName (starts at logon)"
  Write-Host "==> Tip: plug in AC power; agent requests system away-mode to reduce sleep."
} catch {
  Write-Host "==> Scheduled task skipped: $_"
  Write-Host "    Start manually: runeverything"
}

Write-Host ""
Write-Host "==> Installed: $Target"
Write-Host "==> Next steps:"
Write-Host "    1. Pair:       runeverything pair"
Write-Host "    2. Or run:     runeverything"
Write-Host "    3. Keep PC online 24h: avoid Sleep; lock screen is OK (network stays)."
Write-Host ""

try {
  & $Target pair
} catch {
  Write-Host "(pair skipped: $_)"
}
