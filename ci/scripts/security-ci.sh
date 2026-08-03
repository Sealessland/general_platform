#!/usr/bin/env bash
# 安全基线检查入口：当前聚合密钥泄露扫描，后续安全检查可在此扩展。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

bash ci/scripts/scan-secrets.sh

echo "security baseline checks passed"
