#!/usr/bin/env bash
# tail prod 容器日志
# 用法：./scripts/logs.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

docker compose logs -f llm-proxy
