#!/usr/bin/env bash
# 后端 CI 主入口：格式、静态检查、单测/集成测试、覆盖率、构建与运行时基准测试。
# 生成的产物统一落到 ci/artifacts/，供本地复核与 GitHub Actions 上传。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/backend"

ARTIFACT_DIR="$ROOT_DIR/ci/artifacts"
mkdir -p "$ARTIFACT_DIR"

# 固定 Go 构建缓存位置，便于 CI 复现与缓存复用
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"

go mod download

# gofmt 检查：列出未格式化的文件，非空即视为失败
gofmt -l . | tee /tmp/redcart-gofmt.out
if [[ -s /tmp/redcart-gofmt.out ]]; then
  echo "gofmt check failed" >&2
  exit 1
fi

go vet ./...

# 全量单测并输出覆盖率，原始输出写入 artifact；POSTGRES_DSN 只影响集成测试用例
POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" \
  go test ./... -p 1 -coverprofile=coverage.out | tee "$ARTIFACT_DIR/backend-test.txt"
# 构建 API 二进制，验证产物可编译（-buildvcs=false 避免 CI 缺 VCS 元数据时报错）
go build -buildvcs=false -o /tmp/redcart-backend-api ./cmd/api

# PostgreSQL 仓储集成测试（独立于全量测试，便于定位失败）
POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
  RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" \
  go test ./internal/redcart/infrastructure/postgres -v | tee "$ARTIFACT_DIR/backend-postgres-integration.txt"

# PostgreSQL 全链路 HTTP 集成测试（Gin -> 应用层 -> GORM/PostgreSQL）
POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
  RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" \
  go test ./internal/redcart/interfaces/httpapi -run '^TestPostgresHTTP' -count=1 -v \
  | tee "$ARTIFACT_DIR/backend-postgres-http-integration.txt"

# 以下基准测试仅在显式开启 PostgreSQL 集成时执行，避免 CI 无数据库环境下失败
if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  # PostgreSQL HTTP 读写路径基准
  POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
    RUN_POSTGRES_INTEGRATION=1 \
    go test ./internal/redcart/interfaces/httpapi -run '^$' -bench 'BenchmarkHTTPPostgres(OrderPreview|CreateOrder)$' -benchmem -count=1 -benchtime="${POSTGRES_BENCHTIME:-2s}" \
    | tee "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt"

  # 把 ns/op 换算成 QPS，便于在 CI 产物中直接阅读
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

# RabbitMQ 发布与 PostgreSQL outbox -> relay 路径基准，需要同时具备数据库与 RabbitMQ
if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" && -n "${RABBITMQ_ADDR:-}" ]]; then
  POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@127.0.0.1:5432/redcart_test?sslmode=disable}" \
    RUN_POSTGRES_INTEGRATION=1 \
    RABBITMQ_ADDR="${RABBITMQ_ADDR}" \
    RABBITMQ_EXCHANGE="${RABBITMQ_EXCHANGE:-redcart.events.bench}" \
    go test ./internal/event/rabbitmq ./internal/event/outbox -run '^$' -bench 'BenchmarkRabbitMQPublish|BenchmarkPostgresRabbitMQOutboxRelay' -benchmem -count=1 -benchtime="${RABBITMQ_BENCHTIME:-1s}" \
    | tee "$ARTIFACT_DIR/backend-rabbitmq-benchmark.txt"

  # 同上：换算 QPS 便于阅读
  awk '
/^Benchmark(RabbitMQ|PostgresRabbitMQ)/ {
  bench=$1
  ns=$3
  qps=1000000000/ns
  printf "%s qps=%.2f ns_per_op=%s\n", bench, qps, ns
}
' "$ARTIFACT_DIR/backend-rabbitmq-benchmark.txt" | tee "$ARTIFACT_DIR/backend-rabbitmq-qps.txt"
else
  printf 'rabbitmq benchmark skipped: RUN_POSTGRES_INTEGRATION is not 1 or RABBITMQ_ADDR is empty\n' | tee "$ARTIFACT_DIR/backend-rabbitmq-benchmark.txt"
  printf 'rabbitmq qps skipped: RUN_POSTGRES_INTEGRATION is not 1 or RABBITMQ_ADDR is empty\n' | tee "$ARTIFACT_DIR/backend-rabbitmq-qps.txt"
fi

# 覆盖率/测试数量/benchmark 数量门禁与指标产物
RUN_POSTGRES_INTEGRATION="${RUN_POSTGRES_INTEGRATION:-0}" bash "$ROOT_DIR/ci/scripts/backend-test-metrics.sh"
