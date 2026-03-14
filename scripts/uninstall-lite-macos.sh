#!/bin/bash
set -euo pipefail

INSTALL_DIR="/opt/clawpanel-lite"
SERVICE_LABEL="com.clawpanel.lite.service"

[[ $(id -u) -eq 0 ]] || { echo "请使用 sudo 运行卸载脚本" >&2; exit 1; }

launchctl bootout system "/Library/LaunchDaemons/${SERVICE_LABEL}.plist" >/dev/null 2>&1 || true
rm -f "/Library/LaunchDaemons/${SERVICE_LABEL}.plist"
rm -f /usr/local/bin/clawpanel-lite /usr/local/bin/clawlite-openclaw
rm -rf "$INSTALL_DIR"

echo "ClawPanel Lite macOS 已卸载"
