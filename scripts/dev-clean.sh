#!/usr/bin/env bash
# 停止 dev 容器 + 删除 dev 镜像（不删 dev 数据库；如要删数据库手动执行 psql）
# 用法：./scripts/dev-clean.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Stopping + removing llm-proxy:dev image..."
docker compose -f docker-compose.dev.yml down --rmi local

echo "==> Done. dev DB 'llm-proxy-dev' 未删除；如需清理："
echo "    docker exec postgres psql -U <user> -d postgres -c 'DROP DATABASE \"llm-proxy-dev\";'"
