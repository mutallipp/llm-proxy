#!/usr/bin/env bash
# tail dev 容器日志
# 用法：./scripts/dev-logs.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

docker compose -f docker-compose.dev.yml logs -f llm-proxy
