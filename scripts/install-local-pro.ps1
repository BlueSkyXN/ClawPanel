$ErrorActionPreference = "Stop"

$candidate = Get-ChildItem -Path . -Filter "clawpanel-v*-windows-amd64.exe" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $candidate) {
  Write-Error "当前目录未找到匹配的 Pro 构建包：clawpanel-v*-windows-amd64.exe"
  exit 1
}

Write-Host "检测到本地 Pro 构建包：$($candidate.FullName)"
$env:LOCAL_BINARY = $candidate.FullName
powershell -ExecutionPolicy Bypass -File "$PSScriptRoot\install.ps1"
