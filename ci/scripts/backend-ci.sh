#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/backend"

go mod download

gofmt -l . | tee /tmp/redcart-gofmt.out
if [[ -s /tmp/redcart-gofmt.out ]]; then
  echo "gofmt check failed" >&2
  exit 1
fi

go vet ./...

POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" go test ./... -coverprofile=coverage.out
go build ./cmd/api
