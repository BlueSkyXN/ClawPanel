$ErrorActionPreference = "Stop"
$InstallDir = "C:\ClawPanelLite"
$ServiceName = "clawpanel-lite"

sc.exe stop $ServiceName 2>$null | Out-Null
sc.exe delete $ServiceName 2>$null | Out-Null
Remove-Item -Force "C:\Windows\System32\clawlite-openclaw.cmd" -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue
Write-Host "ClawPanel Lite for Windows has been removed."
