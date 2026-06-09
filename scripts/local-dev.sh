#!/usr/bin/env bash
set -euo pipefail

printf 'starting local dependencies with docker compose\n'
docker compose up -d postgres redis pyroscope backend frontend
printf 'backend:  http://127.0.0.1:18080\n'
printf 'frontend: http://127.0.0.1:4173\n'
printf 'postgres: postgresql://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable\n'
printf 'redis:    redis://127.0.0.1:6379\n'
printf 'pyroscope: http://127.0.0.1:4040\n'
