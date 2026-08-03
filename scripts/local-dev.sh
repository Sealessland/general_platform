#!/usr/bin/env bash
set -euo pipefail

# 一键启动本地依赖容器（postgres/redis/pyroscope/backend/frontend）。
# 下方端口与 docker-compose.yml 中的映射保持一致，改端口时需同步本文件。
printf 'starting local dependencies with docker compose\n'
docker compose up -d postgres redis pyroscope backend frontend
printf 'backend:  http://127.0.0.1:18080\n'
printf 'frontend: http://127.0.0.1:4173\n'
printf 'postgres: postgresql://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable\n'
printf 'redis:    redis://127.0.0.1:6380\n'
printf 'pyroscope: http://127.0.0.1:4040\n'
