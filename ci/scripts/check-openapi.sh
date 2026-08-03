#!/usr/bin/env bash
# OpenAPI 契约快速检查：对 docs/api/openapi.yaml 做冒烟级别的关键点校验，
# 覆盖规范版本与核心端点/幂等头，不做完整 schema 校验（完整校验由 lint 工具承担）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

api_file="docs/api/openapi.yaml"

if [[ ! -f "$api_file" ]]; then
  printf 'missing %s\n' "$api_file" >&2
  exit 1
fi

# 契约冒烟点：规范版本、订单/幂等、AI 卖点、商家漏斗等关键路径
grep -q "openapi: 3.0.3" "$api_file"
grep -q "/api/orders:" "$api_file"
grep -q "Idempotency-Key" "$api_file"
grep -q "/api/ai/product-selling-points:" "$api_file"
grep -q "/api/merchant/dashboard/funnel:" "$api_file"

printf 'openapi contract check passed\n'
