#!/usr/bin/env bash
# 全仓库密钥泄露扫描：遍历非忽略目录下的文本文件，匹配常见密钥格式。
# 命中即输出文件与匹配行并最终以非零退出码失败。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

fail=0

# 常见密钥格式：AI 平台 API key、AWS AKIA 密钥、私钥块、键值对形式的凭据
scan_patterns=(
  'sk-[A-Za-z0-9_-]{20,}'
  'AKIA[0-9A-Z]{16}'
  '-----BEGIN (RSA |EC |OPENSSH |)PRIVATE KEY-----'
  '(api[_-]?key|secret|token|password)[[:space:]]*[:=][[:space:]]*["'\'']?[A-Za-z0-9_./+=-]{12,}'
)

# 允许名单：示例文件与脚本自身（脚本内含上述正则，避免自匹配误报）
allowed_files=(
  ".env.example"
  "ci/scripts/scan-secrets.sh"
)

is_allowed_file() {
  local candidate="$1"
  for allowed in "${allowed_files[@]}"; do
    if [[ "$candidate" == "$allowed" ]]; then
      return 0
    fi
  done
  return 1
}

# 遍历仓库文件，跳过 .git、依赖、构建产物与虚拟环境
while IFS= read -r file; do
  if is_allowed_file "$file"; then
    continue
  fi
  for pattern in "${scan_patterns[@]}"; do
    if grep -E -n "$pattern" "$file" >/tmp/redcart-secret-match 2>/dev/null; then
      printf 'possible secret in %s:\n' "$file" >&2
      sed -n '1,5p' /tmp/redcart-secret-match >&2
      fail=1
    fi
  done
done < <(find . -type f \
  -not -path './.git/*' \
  -not -path './frontend/node_modules/*' \
  -not -path './frontend/dist/*' \
  -not -path '*/.venv/*')

rm -f /tmp/redcart-secret-match

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

printf 'secret scan passed\n'
