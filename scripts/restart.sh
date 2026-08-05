#!/usr/bin/env bash
# 重建 prod 容器（不重新 build 镜像；用 .env / config.yml 改动后调用）
# 用法：./scripts/restart.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==> Recreating llm-proxy container..."
docker compose up -d --force-recreate llm-proxy

echo "==> Done. Tail logs: ./scripts/logs.sh"
