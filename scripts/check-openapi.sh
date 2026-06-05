#!/usr/bin/env bash
set -euo pipefail

api_file="docs/api/openapi.yaml"

if [[ ! -f "$api_file" ]]; then
  printf 'missing %s\n' "$api_file" >&2
  exit 1
fi

grep -q "openapi: 3.0.3" "$api_file"
grep -q "/api/orders:" "$api_file"
grep -q "Idempotency-Key" "$api_file"
grep -q "/api/ai/product-selling-points:" "$api_file"
grep -q "/api/merchant/dashboard/funnel:" "$api_file"

printf 'openapi contract check passed\n'
