#!/bin/bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${1:-${VERSION:-0.1.0-alpha}}
IMAGE=${IMAGE:-openclaw-im-manager-openclaw-qq:latest}
OUTPUT_DIR=${OUTPUT_DIR:-"$ROOT_DIR/release/lite/v$VERSION"}

mkdir -p "$OUTPUT_DIR"

echo "==> 导出 Lite QQ Bundle"
echo "    Image: $IMAGE"

docker image inspect "$IMAGE" >/dev/null
docker save "$IMAGE" | gzip -c > "$OUTPUT_DIR/clawpanel-lite-qq-bundle-v$VERSION-linux-amd64.tar.gz"
sha256sum "$OUTPUT_DIR/clawpanel-lite-qq-bundle-v$VERSION-linux-amd64.tar.gz" >> "$OUTPUT_DIR/checksums.txt"

echo "==> Lite QQ Bundle 导出完成"
echo "    $OUTPUT_DIR/clawpanel-lite-qq-bundle-v$VERSION-linux-amd64.tar.gz"
