#!/usr/bin/env bash
# 停止 prod 容器（保留镜像）
# 用法：./scripts/stop.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Stopping llm-proxy..."
docker compose stop llm-proxy

echo "==> Done. Start again: ./scripts/start.sh"
