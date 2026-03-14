@echo off
setlocal ENABLEDELAYEDEXPANSION

set SCRIPT_DIR=%~dp0
for %%I in ("%SCRIPT_DIR%..") do set INSTALL_ROOT=%%~fI
set RUNTIME_ROOT=%INSTALL_ROOT%\runtime
set DATA_ROOT=%INSTALL_ROOT%\data

set NODE_BIN=
for %%I in ("%RUNTIME_ROOT%\node\node.exe" "%RUNTIME_ROOT%\node\bin\node.exe" "%RUNTIME_ROOT%\bin\node.exe") do (
  if exist %%~I (
    set NODE_BIN=%%~I
    goto :node_found
  )
)

echo [clawlite-openclaw] 未找到 Lite 内置 Node.js 运行时 1>&2
exit /b 1

:node_found
set OPENCLAW_APP=
for %%I in ("%RUNTIME_ROOT%\openclaw" "%RUNTIME_ROOT%\openclaw\package" "%RUNTIME_ROOT%\openclaw\app") do (
  if exist %%~I\openclaw.mjs (
    set OPENCLAW_APP=%%~I
    goto :app_found
  )
)

echo [clawlite-openclaw] 未找到 Lite 内置 OpenClaw 入口文件 1>&2
exit /b 1

:app_found
if not exist "%DATA_ROOT%\openclaw-config" mkdir "%DATA_ROOT%\openclaw-config"
if not exist "%DATA_ROOT%\openclaw-work" mkdir "%DATA_ROOT%\openclaw-work"

set OPENCLAW_DIR=%DATA_ROOT%\openclaw-config
set OPENCLAW_STATE_DIR=%DATA_ROOT%\openclaw-config
set OPENCLAW_CONFIG_PATH=%DATA_ROOT%\openclaw-config\openclaw.json
set OPENCLAW_WORK=%DATA_ROOT%\openclaw-work

"%NODE_BIN%" "%OPENCLAW_APP%\openclaw.mjs" %*
