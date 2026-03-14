#!/bin/bash
set -euo pipefail

GITHUB_REPO=${GITHUB_REPO:-zhaoxinyi02/ClawPanel}
GITEE_REPO=${GITEE_REPO:-zxy000006/ClawPanel}
TARGET_ROOT=${TARGET_ROOT:-/data/clawpanel/update}
RELEASE_DIR="$TARGET_ROOT/releases"
SCRIPT_DIR="$TARGET_ROOT/scripts"
PLUGIN_DIR="$TARGET_ROOT/plugins"
BIN_DIR="$TARGET_ROOT/bin/openclaw"
SYNC_ROOT=${SYNC_ROOT:-/tmp/clawpanel-accel-sync}

RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
MAGENTA='\033[35m'
CYAN='\033[36m'
BOLD='\033[1m'
NC='\033[0m'

log(){ echo -e "${GREEN}[Accel Sync]${NC} $1"; }
warn(){ echo -e "${YELLOW}[Accel Sync]${NC} $1"; }
err(){ echo -e "${RED}[Accel Sync]${NC} $1" >&2; exit 1; }

command -v python3 >/dev/null 2>&1 || err "缺少 python3"
command -v curl >/dev/null 2>&1 || err "缺少 curl"

mkdir -p "$RELEASE_DIR" "$SCRIPT_DIR" "$PLUGIN_DIR" "$BIN_DIR" "$SYNC_ROOT"

download_raw() {
  local url="$1"
  local dest="$2"
  curl --http1.1 --progress-bar --retry 3 --retry-delay 2 --connect-timeout 15 --max-time 1800 -fL "$url" -o "$dest"
}

python3 - <<'PY' "$GITHUB_REPO" "$SYNC_ROOT"
import json
import os
import pathlib
import subprocess
import urllib.request

repo, sync_root = os.sys.argv[1:3]
sync_root = pathlib.Path(sync_root)
sync_root.mkdir(parents=True, exist_ok=True)

def version_key(tag):
    tag = tag.split('-', 1)[1]
    tag = tag.lstrip('v')
    out = []
    for part in tag.replace('-', '.').split('.'):
        if part.isdigit():
            out.append((0, int(part)))
        else:
            out.append((1, part))
    return out

with urllib.request.urlopen(f'https://api.github.com/repos/{repo}/releases?per_page=50') as resp:
    releases = json.load(resp)

for prefix in ('pro-v', 'lite-v'):
    candidates = [r for r in releases if r.get('tag_name', '').startswith(prefix)]
    if not candidates:
        continue
    latest = sorted(candidates, key=lambda r: version_key(r['tag_name']), reverse=True)[0]
    target = sync_root / latest['tag_name']
    target.mkdir(parents=True, exist_ok=True)
    for asset in latest.get('assets', []):
        out = target / asset['name']
        if out.exists() and out.stat().st_size == asset['size']:
            continue
        subprocess.check_call([
            'curl','--http1.1','--progress-bar','--retry','3','--retry-delay','2',
            '--connect-timeout','15','--max-time','7200','-fL',
            asset['browser_download_url'],'-o',str(out)
        ])
PY

latest_pro=$(find "$SYNC_ROOT" -maxdepth 1 -type d -name 'pro-v*' | sort -V | tail -n 1)
latest_lite=$(find "$SYNC_ROOT" -maxdepth 1 -type d -name 'lite-v*' | sort -V | tail -n 1)

[ -n "$latest_pro" ] || err "未找到最新 Pro Release"
[ -n "$latest_lite" ] || err "未找到最新 Lite Release"

log "同步最新 Pro Release: $(basename "$latest_pro")"
cp -f "$latest_pro"/* "$RELEASE_DIR/"
log "同步最新 Lite Release: $(basename "$latest_lite")"
cp -f "$latest_lite"/* "$RELEASE_DIR/"

log "同步更新元数据与插件注册表..."
download_raw "https://gitee.com/${GITEE_REPO}/raw/main/release/update-pro.json" "$TARGET_ROOT/update-pro.json"
download_raw "https://gitee.com/${GITEE_REPO}/raw/main/release/update-lite.json" "$TARGET_ROOT/update-lite.json"
download_raw "https://gitee.com/${GITEE_REPO}/raw/main/release/update.json" "$TARGET_ROOT/update.json"
download_raw "https://gitee.com/${GITEE_REPO}/raw/main/plugins/registry.json" "$PLUGIN_DIR/registry.json"

log "同步安装/卸载脚本..."
for f in install.sh install-pro.sh install-lite.sh install-lite-macos.sh install-lite.ps1 install.ps1 uninstall-lite.sh uninstall-lite-macos.sh uninstall-lite.ps1 install-local-pro.sh install-local-lite.sh install-local-pro.ps1 install-local-lite.ps1; do
  download_raw "https://gitee.com/${GITEE_REPO}/raw/main/scripts/${f}" "$SCRIPT_DIR/${f}"
done

if [ ! -f "$BIN_DIR/openclaw-2026.2.26-linux-x64-prefix.tar.gz" ]; then
  warn "未自动同步 openclaw-2026.2.26-linux-x64-prefix.tar.gz，如需 Linux 离线 OpenClaw 安装，请手动补传到 $BIN_DIR"
fi

chmod 755 "$SCRIPT_DIR"/*.sh

log "同步完成"
log "Releases: $RELEASE_DIR"
log "Scripts:  $SCRIPT_DIR"
log "Update JSON: $TARGET_ROOT/update-pro.json / update-lite.json"
