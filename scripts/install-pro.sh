#!/bin/bash
set -euo pipefail

ACCEL_BASE="http://39.102.53.188:16198/clawpanel"
TMP_SCRIPT=$(mktemp)
trap 'rm -f "$TMP_SCRIPT"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$ACCEL_BASE/scripts/install.sh" -o "$TMP_SCRIPT"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP_SCRIPT" "$ACCEL_BASE/scripts/install.sh"
else
  echo "缺少 curl/wget，无法下载 Pro 安装脚本" >&2
  exit 1
fi

chmod +x "$TMP_SCRIPT"
UPDATE_META=update-pro.json bash "$TMP_SCRIPT" "$@"
