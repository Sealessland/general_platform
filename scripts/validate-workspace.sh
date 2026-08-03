#!/usr/bin/env bash
set -euo pipefail

# 根级转发入口：仓库结构/内容校验逻辑在 ci/scripts/validate-workspace.sh，
# 与 CI（.github/workflows/ci.yml）执行同一套校验。
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec bash "$ROOT_DIR/ci/scripts/validate-workspace.sh"
