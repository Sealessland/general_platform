#!/usr/bin/env bash
set -euo pipefail

printf 'starting local dependencies with docker compose\n'
docker compose up -d postgres redis pyroscope backend backend-replica gateway frontend
printf 'gateway:  http://127.0.0.1:18080 (load balances backend :18081 and :18082)\n'
printf 'frontend: http://127.0.0.1:4173\n'
printf 'postgres: postgresql://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable\n'
printf 'redis:    redis://127.0.0.1:6380\n'
printf 'kafka:    127.0.0.1:19092 (topic: redcart.events)\n'
printf 'pyroscope: http://127.0.0.1:4040\n'
