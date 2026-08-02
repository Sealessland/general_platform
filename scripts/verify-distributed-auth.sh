#!/usr/bin/env bash
set -euo pipefail

primary="${PRIMARY_API:-http://127.0.0.1:18081}"
replica="${REPLICA_API:-http://127.0.0.1:18082}"

for base_url in "$primary" "$replica"; do
  curl --fail --silent --show-error "$base_url/healthz" >/dev/null
done

login_json="$({
  curl --fail --silent --show-error \
    -H 'Content-Type: application/json' \
    -d '{"phone":"13800000001","password":"consumer-demo"}' \
    "$primary/api/auth/login"
})"
access_token="$(python3 -c 'import json, sys; print(json.load(sys.stdin)["token"])' <<<"$login_json")"

curl --fail --silent --show-error \
  -H "Authorization: Bearer $access_token" \
  "$replica/api/auth/me" >/dev/null

curl --fail --silent --show-error \
  -X POST \
  -H "Authorization: Bearer $access_token" \
  "$replica/api/auth/logout" >/dev/null

status="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  -H "Authorization: Bearer $access_token" \
  "$primary/api/auth/me")"
if [[ "$status" != "401" ]]; then
  printf 'expected primary to observe replica logout, got HTTP %s\n' "$status" >&2
  exit 1
fi

printf 'distributed auth verified: login on :18081, authenticate/logout on :18082, revocation observed on :18081\n'
