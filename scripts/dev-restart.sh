#!/usr/bin/env bash
# 重建 dev 容器（不重新 build 镜像）
# 用法：./scripts/dev-restart.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Recreating llm-proxy-dev container..."
docker compose -f docker-compose.dev.yml up -d --force-recreate llm-proxy

echo "==> Done. Tail logs: ./scripts/dev-logs.sh"
