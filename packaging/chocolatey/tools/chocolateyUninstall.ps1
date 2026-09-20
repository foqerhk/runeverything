$ErrorActionPreference = 'Stop'
$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$exe = Join-Path $toolsDir 'runeverything.exe'
if (Test-Path $exe) {
  Uninstall-BinFile -Name 'runeverything' -Path $exe
}
