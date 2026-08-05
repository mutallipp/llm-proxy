#!/usr/bin/env bash
# 启动 prod 容器（端口 8090，使用 docker-compose.yml）
# 用法：./scripts/start.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Building llm-proxy image..."
docker compose build llm-proxy

echo "==> Starting llm-proxy (port 8090)..."
docker compose up -d llm-proxy

echo "==> Done. Tail logs: ./scripts/logs.sh"
