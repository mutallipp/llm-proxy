#!/usr/bin/env bash
# 启动前端 Vite dev server（端口 15173，代理到 18090 后端）
# 用法：./scripts/dev-frontend.sh
# 前置：./scripts/dev-up.sh 已运行；当前目录有 frontend/ + pnpm
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ ! -d frontend ]; then
  echo "ERROR: frontend/ 不存在" >&2
  exit 1
fi

cd frontend
VITE_API_URL=http://localhost:18090 pnpm dev --port 15173
