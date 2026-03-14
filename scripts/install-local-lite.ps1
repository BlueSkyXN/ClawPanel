$ErrorActionPreference = "Stop"

$candidate = Get-ChildItem -Path . -Filter "clawpanel-lite-core-v*-windows-amd64.tar.gz" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $candidate) {
  Write-Error "当前目录未找到匹配的 Lite 构建包：clawpanel-lite-core-v*-windows-amd64.tar.gz"
  exit 1
}

Write-Host "检测到本地 Lite 构建包：$($candidate.FullName)"
$env:LOCAL_PACKAGE = $candidate.FullName
powershell -ExecutionPolicy Bypass -File "$PSScriptRoot\install-lite.ps1"
