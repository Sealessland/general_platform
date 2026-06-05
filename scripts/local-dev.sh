#!/usr/bin/env bash
set -euo pipefail

printf 'starting local dependencies with docker compose\n'
docker compose up -d postgres redis rabbitmq
printf 'starting backend api on http://127.0.0.1:18080\n'
(cd backend && HTTP_PORT=18080 GOCACHE=/tmp/go-build-cache go run ./cmd/api)
