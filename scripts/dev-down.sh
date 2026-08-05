#!/usr/bin/env bash
# 停止并删除 dev 容器
# 用法：./scripts/dev-down.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Stopping llm-proxy-dev..."
docker compose -f docker-compose.dev.yml down

echo "==> Done. Start again: ./scripts/dev-up.sh"
