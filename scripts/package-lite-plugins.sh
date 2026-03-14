#!/bin/bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR=${1:-"$ROOT_DIR/lite-assets/plugins"}
GLOBAL_NODE_ROOT=${GLOBAL_NODE_ROOT:-}
OPENCLAW_CONFIG_ROOT=${OPENCLAW_CONFIG_ROOT:-"$HOME/openclaw/config"}
PLUGIN_REPO_ROOT=${PLUGIN_REPO_ROOT:-"$HOME/ClawPanel-Plugins/official"}
OPENCLAW_APP_ROOT=${OPENCLAW_APP_ROOT:-"/usr/lib/node_modules/openclaw"}
TARGET_OS=${TARGET_OS:-linux}
TMP_NPM_DIR=${TMP_NPM_DIR:-$(mktemp -d)}

cleanup() {
  rm -rf "$TMP_NPM_DIR"
}
trap cleanup EXIT

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

first_existing_dir() {
  local candidate
  for candidate in "$@"; do
    if [[ -n "$candidate" && -d "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

copy_dir() {
  local name=$1
  shift
  local src
  if ! src=$(first_existing_dir "$@"); then
    echo "跳过插件: $name (未找到候选目录)" >&2
    return 0
  fi
  rm -rf "$OUT_DIR/$name"
  cp -a "$src" "$OUT_DIR/$name"
  echo "已收集插件: $name <- $src"
}

copy_npm_pkg() {
  local install_spec=$1
  local package_dir=$2
  local target_name=$3
  local src="$TMP_NPM_DIR/node_modules/$package_dir"
  if [[ ! -d "$src" ]]; then
    echo "跳过插件: $target_name (未找到 npm 安装目录 $src)" >&2
    return 0
  fi
  rm -rf "$OUT_DIR/$target_name"
  cp -a "$src" "$OUT_DIR/$target_name"
  echo "已收集插件: $target_name <- $install_spec"
}

rewrite_wecom_manifest() {
  python3 - <<'PY' "$OUT_DIR/wecom/package.json" "$OUT_DIR/wecom/openclaw.plugin.json"
import json, sys
package_path, manifest_path = sys.argv[1:3]
with open(package_path, 'r', encoding='utf-8') as f:
    pkg = json.load(f)
pkg.setdefault('openclaw', {}).setdefault('channel', {})['id'] = 'wecom'
pkg['openclaw'].setdefault('install', {})['localPath'] = 'extensions/wecom'
with open(package_path, 'w', encoding='utf-8') as f:
    json.dump(pkg, f, ensure_ascii=False, indent=2)
    f.write('\n')
with open(manifest_path, 'r', encoding='utf-8') as f:
    manifest = json.load(f)
manifest['id'] = 'wecom'
with open(manifest_path, 'w', encoding='utf-8') as f:
    json.dump(manifest, f, ensure_ascii=False, indent=2)
    f.write('\n')
PY
}

if [[ -z "$GLOBAL_NODE_ROOT" ]]; then
  for candidate in \
    "/usr/lib/node_modules" \
    "/usr/local/lib/node_modules" \
    "$HOME/.npm-global/lib/node_modules" \
    "$HOME/.local/lib/node_modules"; do
    if [[ -d "$candidate" ]]; then
      GLOBAL_NODE_ROOT="$candidate"
      break
    fi
  done
fi

echo "==> 安装 Lite 预置插件候选 npm 包"
npm install --omit=dev --registry=https://registry.npmmirror.com --prefix "$TMP_NPM_DIR" \
  clawdbot-dingtalk@0.4.6 \
  @sliverp/qqbot@1.5.3 \
  @wecom/wecom-openclaw-plugin@1.0.6 >/dev/null

if [[ "$TARGET_OS" == "linux" ]]; then
  copy_dir "qq" \
    "$PLUGIN_REPO_ROOT/qq" \
    "$OPENCLAW_CONFIG_ROOT/extensions/qq"
fi
copy_npm_pkg "@sliverp/qqbot@1.5.3" "@sliverp/qqbot" "qqbot"
copy_npm_pkg "@wecom/wecom-openclaw-plugin@1.0.6" "@wecom/wecom-openclaw-plugin" "wecom"
if [[ -f "$OUT_DIR/wecom/openclaw.plugin.json" ]]; then
  rewrite_wecom_manifest
fi
if [[ "$TARGET_OS" == "linux" ]]; then
  copy_dir "wecom-app" \
    "$GLOBAL_NODE_ROOT/@openclaw-china/wecom-app" \
    "$OPENCLAW_CONFIG_ROOT/extensions/wecom-app"
  copy_dir "dingtalk" \
    "$OPENCLAW_CONFIG_ROOT/extensions/dingtalk" \
    "$PLUGIN_REPO_ROOT/dingtalk" \
    "$OPENCLAW_CONFIG_ROOT/extensions/dingtalk"
else
  copy_npm_pkg "clawdbot-dingtalk@0.4.6" "clawdbot-dingtalk" "dingtalk"
fi

echo "Lite 预置插件目录已输出到: $OUT_DIR"
