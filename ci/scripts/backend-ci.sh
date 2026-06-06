#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/backend"

ARTIFACT_DIR="$ROOT_DIR/ci/artifacts"
mkdir -p "$ARTIFACT_DIR"

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"

go mod download

gofmt -l . | tee /tmp/redcart-gofmt.out
if [[ -s /tmp/redcart-gofmt.out ]]; then
  echo "gofmt check failed" >&2
  exit 1
fi

go vet ./...

POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" go test ./... -coverprofile=coverage.out
go build ./cmd/api

POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
  RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" \
  go test ./internal/redcart/infrastructure/postgres -v | tee "$ARTIFACT_DIR/backend-postgres-integration.txt"

POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
  RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" \
  go test ./internal/redcart/interfaces/httpapi -run '^TestPostgresHTTP' -count=1 -v \
  | tee "$ARTIFACT_DIR/backend-postgres-http-integration.txt"

POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
  go test ./internal/redcart/interfaces/httpapi -run '^$' -bench 'BenchmarkHTTP(Notes|OrderPreview)$' -benchmem -count=1 \
  | tee "$ARTIFACT_DIR/backend-benchmark.txt"

awk '
/^BenchmarkHTTP/ {
  bench=$1
  ns=$3
  qps=1000000000/ns
  printf "%s qps=%.2f ns_per_op=%s\n", bench, qps, ns
}
' "$ARTIFACT_DIR/backend-benchmark.txt" | tee "$ARTIFACT_DIR/backend-qps.txt"

if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
    RUN_POSTGRES_INTEGRATION=1 \
    go test ./internal/redcart/interfaces/httpapi -run '^$' -bench 'BenchmarkHTTPPostgres(OrderPreview|CreateOrder)$' -benchmem -count=1 -benchtime="${POSTGRES_BENCHTIME:-2s}" \
    | tee "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt"

  awk '
/^BenchmarkHTTPPostgres/ {
  bench=$1
  ns=$3
  qps=1000000000/ns
  printf "%s qps=%.2f ns_per_op=%s\n", bench, qps, ns
}
' "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt" | tee "$ARTIFACT_DIR/backend-postgres-http-qps.txt"
else
  printf 'postgres http benchmark skipped: RUN_POSTGRES_INTEGRATION is not 1\n' | tee "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt"
  printf 'postgres http qps skipped: RUN_POSTGRES_INTEGRATION is not 1\n' | tee "$ARTIFACT_DIR/backend-postgres-http-qps.txt"
fi
