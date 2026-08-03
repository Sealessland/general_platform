#!/usr/bin/env bash
set -euo pipefail

# 根级转发入口：OpenAPI 契约校验逻辑在 ci/scripts/check-openapi.sh，
# 本地开发与 CI 共用同一实现，避免两处逻辑漂移。
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec bash "$ROOT_DIR/ci/scripts/check-openapi.sh"
