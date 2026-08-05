#!/usr/bin/env bash
# 启动 dev 容器（端口 18090，使用 docker-compose.dev.yml + .env.dev）
# 首次运行前需手动 CREATE DATABASE "llm-proxy-dev"
# 用法：./scripts/dev-up.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ ! -f .env.dev ]; then
  echo "ERROR: .env.dev 不存在；从 .env.example 复制并改名为 .env.dev" >&2
  exit 1
fi

echo "==> Building llm-proxy:dev image..."
docker compose -f docker-compose.dev.yml build llm-proxy

echo "==> Starting llm-proxy-dev (port 18090)..."
docker compose -f docker-compose.dev.yml up -d llm-proxy

echo "==> Done. Tail logs: ./scripts/dev-logs.sh   Frontend: ./scripts/dev-frontend.sh"
