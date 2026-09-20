$ErrorActionPreference = 'Stop'
$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$version = '0.1.0'
$packageName = 'runeverything'

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$url = "https://github.com/foqerhk/runeverything/releases/download/v$version/runeverything_windows_$arch.zip"

# Updated by scripts/sync-chocolatey.sh
$checksumAmd = 'd90bc02b433767e2150b1e8d48f968cfe0808cd456643ca638422721c11a4ae9'
$checksumArm = '14b2d081d97c92a1a297b401c848796591eb4bbbb1c26cd547df5621269799d6'
$checksumType = 'sha256'
$checksum = if ($arch -eq 'arm64') { $checksumArm } else { $checksumAmd }

Install-ChocolateyZipPackage `
  -PackageName $packageName `
  -Url64bit $url `
  -Checksum64 $checksum `
  -ChecksumType64 $checksumType `
  -UnzipLocation $toolsDir

$exe = Join-Path $toolsDir 'runeverything.exe'
Install-BinFile -Name 'runeverything' -Path $exe

Write-Host "Pair with: runeverything pair"
Write-Host "Set relay:  `$env:RE_RELAY = 'wss://your-relay.example/re2'"
