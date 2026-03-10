#!/bin/bash
set -euo pipefail

INSTALL_DIR="/opt/clawpanel-lite"
SERVICE_NAME="clawpanel-lite"

if [[ $(id -u) -ne 0 ]]; then
  echo "请使用 root 或 sudo 执行卸载" >&2
  exit 1
fi

systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload

rm -f /usr/local/bin/clawpanel-lite
rm -f /usr/local/bin/clawlite-openclaw
rm -rf "$INSTALL_DIR"

echo "ClawPanel Lite 已卸载"
echo "- 已删除服务: $SERVICE_NAME"
echo "- 已删除目录: $INSTALL_DIR"
echo "- 已删除命令: /usr/local/bin/clawpanel-lite, /usr/local/bin/clawlite-openclaw"
