#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

detect_os() {
  case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
    linux) echo linux ;;
    darwin) echo darwin ;;
    *) echo unsupported ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) echo unsupported ;;
  esac
}

SYS_OS=$(detect_os)
SYS_ARCH=$(detect_arch)

if [[ "$SYS_OS" == unsupported || "$SYS_ARCH" == unsupported ]]; then
  echo "不支持的系统平台：$(uname -s)/$(uname -m)" >&2
  exit 1
fi

pattern="clawpanel-lite-core-v*-${SYS_OS}-${SYS_ARCH}.tar.gz"
candidate=$(ls -t ./$pattern 2>/dev/null | head -n 1 || true)

if [[ -z "${candidate:-}" || ! -f "$candidate" ]]; then
  echo "当前目录未找到匹配的 Lite 构建包：$pattern" >&2
  exit 1
fi

echo "检测到本地 Lite 构建包：$candidate"
if [[ "$SYS_OS" == linux ]]; then
  sudo LOCAL_PACKAGE="$(pwd)/${candidate#./}" bash "$SCRIPT_DIR/install-lite.sh"
elif [[ "$SYS_OS" == darwin ]]; then
  sudo LOCAL_PACKAGE="$(pwd)/${candidate#./}" bash "$SCRIPT_DIR/install-lite-macos.sh"
else
  echo "Windows 请使用 install-local-lite.ps1" >&2
  exit 1
fi
