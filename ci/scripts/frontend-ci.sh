#!/usr/bin/env bash
# 前端 CI 主入口：安装依赖后依次执行类型检查、lint、单元测试与构建。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/frontend"

# npm ci 按 lockfile 精确安装，保证 CI 与本地依赖一致
npm ci
npm run typecheck
npm run lint
npm test
npm run build
