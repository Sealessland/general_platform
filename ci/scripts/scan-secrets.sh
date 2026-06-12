#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

fail=0

scan_patterns=(
  'sk-[A-Za-z0-9_-]{20,}'
  'AKIA[0-9A-Z]{16}'
  '-----BEGIN (RSA |EC |OPENSSH |)PRIVATE KEY-----'
  '(api[_-]?key|secret|token|password)[[:space:]]*[:=][[:space:]]*["'\'']?[A-Za-z0-9_./+=-]{12,}'
)

allowed_files=(
  ".env.example"
  "scripts/scan-secrets.sh"
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
