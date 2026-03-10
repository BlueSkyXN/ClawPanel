#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
INSTALL_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
RUNTIME_ROOT="$INSTALL_ROOT/runtime"
DATA_ROOT="$INSTALL_ROOT/data"

NODE_BIN=""
for candidate in \
  "$RUNTIME_ROOT/node/bin/node" \
  "$RUNTIME_ROOT/node/node" \
  "$RUNTIME_ROOT/bin/node"
do
  if [ -x "$candidate" ]; then
    NODE_BIN="$candidate"
    break
  fi
done

OPENCLAW_APP=""
for candidate in \
  "$RUNTIME_ROOT/openclaw" \
  "$RUNTIME_ROOT/openclaw/package" \
  "$RUNTIME_ROOT/openclaw/app"
do
  if [ -f "$candidate/openclaw.mjs" ]; then
    OPENCLAW_APP="$candidate"
    break
  fi
done

if [ -z "$NODE_BIN" ]; then
  echo "[clawlite-openclaw] 未找到 Lite 内置 Node.js 运行时" >&2
  exit 1
fi

if [ -z "$OPENCLAW_APP" ]; then
  echo "[clawlite-openclaw] 未找到 Lite 内置 OpenClaw 入口文件" >&2
  exit 1
fi

mkdir -p "$DATA_ROOT/openclaw-config" "$DATA_ROOT/openclaw-work"

export OPENCLAW_DIR="$DATA_ROOT/openclaw-config"
export OPENCLAW_STATE_DIR="$DATA_ROOT/openclaw-config"
export OPENCLAW_CONFIG_PATH="$DATA_ROOT/openclaw-config/openclaw.json"
export OPENCLAW_WORK="$DATA_ROOT/openclaw-work"

exec "$NODE_BIN" "$OPENCLAW_APP/openclaw.mjs" "$@"
