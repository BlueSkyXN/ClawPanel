#!/bin/bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${1:-${VERSION:-0.1.0-alpha}}
OUTPUT_DIR=${OUTPUT_DIR:-"$ROOT_DIR/release/lite/v$VERSION"}
STAGE_DIR=$(mktemp -d)

cleanup() {
  rm -rf "$STAGE_DIR"
}
trap cleanup EXIT

NODE_BIN=${NODE_BIN:-$(command -v node || true)}
OPENCLAW_SRC=${OPENCLAW_SRC:-}
PLUGIN_ROOT=${PLUGIN_ROOT:-}

if [[ -z "$NODE_BIN" ]]; then
  echo "未找到 node，可通过 NODE_BIN=/path/to/node 指定" >&2
  exit 1
fi

if [[ -z "$OPENCLAW_SRC" ]]; then
  for candidate in \
    "/usr/lib/node_modules/openclaw" \
    "/usr/local/lib/node_modules/openclaw" \
    "$HOME/.npm-global/lib/node_modules/openclaw"; do
    if [[ -f "$candidate/package.json" ]]; then
      OPENCLAW_SRC="$candidate"
      break
    fi
  done
fi

if [[ -z "$OPENCLAW_SRC" || ! -f "$OPENCLAW_SRC/package.json" ]]; then
  echo "未找到 OpenClaw 安装目录，可通过 OPENCLAW_SRC=/path/to/openclaw 指定" >&2
  exit 1
fi

PLUGIN_ROOT=${PLUGIN_ROOT:-"$ROOT_DIR/lite-assets/plugins"}

echo "==> 打包 Lite Core v$VERSION"
echo "    Node:      $NODE_BIN"
echo "    OpenClaw:  $OPENCLAW_SRC"
echo "    Plugins:   $PLUGIN_ROOT"

mkdir -p "$OUTPUT_DIR"
mkdir -p "$STAGE_DIR/runtime" "$STAGE_DIR/data/openclaw-config" "$STAGE_DIR/data/openclaw-work" "$STAGE_DIR/bin" "$STAGE_DIR/.plugin-build"

cp "$ROOT_DIR/bin/clawpanel-lite" "$STAGE_DIR/clawpanel-lite"
cp "$ROOT_DIR/scripts/clawlite-openclaw.sh" "$STAGE_DIR/bin/clawlite-openclaw"
chmod +x "$STAGE_DIR/clawpanel-lite" "$STAGE_DIR/bin/clawlite-openclaw"

mkdir -p "$STAGE_DIR/runtime/node/bin"
cp -a "$NODE_BIN" "$STAGE_DIR/runtime/node/bin/node"
cp -a "$OPENCLAW_SRC" "$STAGE_DIR/runtime/openclaw"

if [[ -d "$PLUGIN_ROOT" ]]; then
  mkdir -p "$STAGE_DIR/runtime/openclaw/extensions"
  for plugin_id in qq qqbot dingtalk wecom wecom-app; do
    if [[ -d "$PLUGIN_ROOT/$plugin_id" ]]; then
      plugin_build_dir="$STAGE_DIR/.plugin-build/$plugin_id"
      rm -rf "$plugin_build_dir" "$STAGE_DIR/runtime/openclaw/extensions/$plugin_id"
      cp -a "$PLUGIN_ROOT/$plugin_id" "$plugin_build_dir"
      if [[ -f "$plugin_build_dir/package.json" && "$plugin_id" != "wecom-app" ]]; then
        echo "==> 安装 Lite 插件依赖: $plugin_id"
	      rm -rf "$plugin_build_dir/node_modules" "$plugin_build_dir/package-lock.json"
	      (cd "$plugin_build_dir" && npm install --omit=dev --omit=peer --no-package-lock --registry=https://registry.npmmirror.com >/dev/null)
      fi
      cp -a "$plugin_build_dir" "$STAGE_DIR/runtime/openclaw/extensions/$plugin_id"
    fi
  done
fi

cat > "$STAGE_DIR/data/openclaw-config/openclaw.json" <<'EOF'
{
  "gateway": {
    "mode": "local",
    "port": 18790
  }
}
EOF

tar -C "$STAGE_DIR" -czf "$OUTPUT_DIR/clawpanel-lite-core-v$VERSION-linux-amd64.tar.gz" .
sha256sum "$OUTPUT_DIR/clawpanel-lite-core-v$VERSION-linux-amd64.tar.gz" > "$OUTPUT_DIR/checksums.txt"

echo "==> Lite Core 打包完成"
echo "    $OUTPUT_DIR/clawpanel-lite-core-v$VERSION-linux-amd64.tar.gz"
