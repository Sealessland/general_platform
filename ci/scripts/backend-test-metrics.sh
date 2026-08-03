#!/usr/bin/env bash
# 后端质量门禁：解析 go test 日志与覆盖率文件，校验覆盖率/测试数量/benchmark 数量阈值，
# 并输出人类可读摘要（backend-coverage-summary.txt）与机器可读指标（backend-test-metrics.json）。
# 所有阈值均可用同名环境变量覆盖，默认值按当前 MVP 的可稳定通过水平设置。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
ARTIFACT_DIR="$ROOT_DIR/ci/artifacts"
mkdir -p "$ARTIFACT_DIR"

COVERPROFILE="${COVERPROFILE:-$BACKEND_DIR/coverage.out}"
BACKEND_TEST_LOG="${BACKEND_TEST_LOG:-$ARTIFACT_DIR/backend-test.txt}"
COVER_FUNCTIONS="$ARTIFACT_DIR/backend-coverage-functions.txt"
COVER_SUMMARY="$ARTIFACT_DIR/backend-coverage-summary.txt"
TEST_LIST="$ARTIFACT_DIR/backend-test-list.txt"
METRICS_JSON="$ARTIFACT_DIR/backend-test-metrics.json"

# 覆盖率与数量门禁阈值（可用环境变量覆盖）
MIN_TOTAL_COVERAGE="${MIN_TOTAL_COVERAGE:-65.0}"
MIN_APPLICATION_COVERAGE="${MIN_APPLICATION_COVERAGE:-80.0}"
MIN_HTTPAPI_COVERAGE="${MIN_HTTPAPI_COVERAGE:-60.0}"
MIN_AI_COVERAGE="${MIN_AI_COVERAGE:-95.0}"
MIN_DOMAIN_COVERAGE="${MIN_DOMAIN_COVERAGE:-95.0}"
MIN_BACKEND_TEST_COUNT="${MIN_BACKEND_TEST_COUNT:-55}"
MIN_POSTGRES_BENCHMARK_COUNT="${MIN_POSTGRES_BENCHMARK_COUNT:-2}"
MIN_RABBITMQ_BENCHMARK_COUNT="${MIN_RABBITMQ_BENCHMARK_COUNT:-2}"

# PostgreSQL 仓储覆盖率：开启集成测试时阈值提高（真实数据库路径才有意义）
if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  MIN_POSTGRES_REPOSITORY_COVERAGE="${MIN_POSTGRES_REPOSITORY_COVERAGE:-75.0}"
else
  MIN_POSTGRES_REPOSITORY_COVERAGE="${MIN_POSTGRES_REPOSITORY_COVERAGE:-15.0}"
fi

cd "$BACKEND_DIR"

# 依赖上游 backend-ci.sh 的产物，缺失时直接失败
if [[ ! -f "$COVERPROFILE" ]]; then
  printf 'missing coverage profile: %s\n' "$COVERPROFILE" >&2
  exit 1
fi
if [[ ! -f "$BACKEND_TEST_LOG" ]]; then
  printf 'missing backend test log: %s\n' "$BACKEND_TEST_LOG" >&2
  exit 1
fi

# 函数级覆盖率清单（含 total 汇总行）
go tool cover -func="$COVERPROFILE" | tee "$COVER_FUNCTIONS" >/dev/null

# 从 go test 日志中提取指定包的语句覆盖率百分比（形如 "ok pkg ... coverage: 82.8% of statements"）
coverage_for_package() {
  local package_path="$1"
  awk -v package_path="$package_path" '
    $1 == "ok" && $2 == package_path {
      for (i = 1; i <= NF; i++) {
        if ($i == "coverage:") {
          value = $(i + 1)
          gsub("%", "", value)
          print value
          found = 1
          exit
        }
      }
    }
    END {
      if (!found) {
        exit 1
      }
    }
  ' "$BACKEND_TEST_LOG"
}

# 数值门禁：value 低于 minimum 时输出错误到 stderr 并返回非零退出码
require_number_at_least() {
  local name="$1"
  local value="$2"
  local minimum="$3"
  awk -v name="$name" -v value="$value" -v minimum="$minimum" '
    BEGIN {
      if ((value + 0) < (minimum + 0)) {
        printf "%s below threshold: value=%s minimum=%s\n", name, value, minimum > "/dev/stderr"
        exit 1
      }
    }
  '
}

# 提取各类覆盖率指标（total 来自 go tool cover 的汇总行，其余来自包级测试日志）
total_coverage="$(awk '/^total:/ {value=$3; gsub("%", "", value); print value}' "$COVER_FUNCTIONS")"
application_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/application")"
httpapi_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi")"
postgres_repository_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres")"
ai_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/ai")"
domain_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/domain")"

# 测试规模：go test -list 枚举全部 Test* 用例并计数
test_count="$(go test ./... -list '^Test' | tee "$TEST_LIST" | awk '/^Test/ {count++} END {print count + 0}')"
# benchmark 数量从 backend-ci.sh 落盘的产物中统计；文件缺失按 0 处理
postgres_benchmark_count="$(awk '/^BenchmarkHTTPPostgres/ {count++} END {print count + 0}' "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt" 2>/dev/null || printf '0')"
rabbitmq_benchmark_count="$(awk '/^Benchmark(RabbitMQ|PostgresRabbitMQ)/ {count++} END {print count + 0}' "$ARTIFACT_DIR/backend-rabbitmq-benchmark.txt" 2>/dev/null || printf '0')"

# 逐一执行门禁检查，任一失败即整体失败
require_number_at_least "total coverage" "$total_coverage" "$MIN_TOTAL_COVERAGE"
require_number_at_least "application package coverage" "$application_coverage" "$MIN_APPLICATION_COVERAGE"
require_number_at_least "httpapi package coverage" "$httpapi_coverage" "$MIN_HTTPAPI_COVERAGE"
require_number_at_least "postgres repository package coverage" "$postgres_repository_coverage" "$MIN_POSTGRES_REPOSITORY_COVERAGE"
require_number_at_least "ai package coverage" "$ai_coverage" "$MIN_AI_COVERAGE"
require_number_at_least "domain package coverage" "$domain_coverage" "$MIN_DOMAIN_COVERAGE"
require_number_at_least "backend test count" "$test_count" "$MIN_BACKEND_TEST_COUNT"
# benchmark 门禁仅在开启 PostgreSQL 集成时生效；RabbitMQ 还要求提供连接地址
if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  require_number_at_least "postgres benchmark count" "$postgres_benchmark_count" "$MIN_POSTGRES_BENCHMARK_COUNT"
  if [[ -n "${RABBITMQ_ADDR:-}" ]]; then
    require_number_at_least "rabbitmq benchmark count" "$rabbitmq_benchmark_count" "$MIN_RABBITMQ_BENCHMARK_COUNT"
  fi
fi

# 人类可读摘要
cat >"$COVER_SUMMARY" <<EOF
backend quality metrics
total_coverage=$total_coverage threshold=$MIN_TOTAL_COVERAGE
application_coverage=$application_coverage threshold=$MIN_APPLICATION_COVERAGE
httpapi_coverage=$httpapi_coverage threshold=$MIN_HTTPAPI_COVERAGE
postgres_repository_coverage=$postgres_repository_coverage threshold=$MIN_POSTGRES_REPOSITORY_COVERAGE
ai_coverage=$ai_coverage threshold=$MIN_AI_COVERAGE
domain_coverage=$domain_coverage threshold=$MIN_DOMAIN_COVERAGE
test_count=$test_count threshold=$MIN_BACKEND_TEST_COUNT
postgres_benchmark_count=$postgres_benchmark_count threshold=$MIN_POSTGRES_BENCHMARK_COUNT run_postgres_integration=${RUN_POSTGRES_INTEGRATION:-0}
rabbitmq_benchmark_count=$rabbitmq_benchmark_count threshold=$MIN_RABBITMQ_BENCHMARK_COUNT rabbitmq_addr_set=$([[ -n "${RABBITMQ_ADDR:-}" ]] && printf true || printf false)
EOF

# 机器可读指标（供外部消费/展示）
cat >"$METRICS_JSON" <<EOF
{
  "total_coverage": $total_coverage,
  "coverage_threshold": $MIN_TOTAL_COVERAGE,
  "packages": {
    "application": {"coverage": $application_coverage, "threshold": $MIN_APPLICATION_COVERAGE},
    "httpapi": {"coverage": $httpapi_coverage, "threshold": $MIN_HTTPAPI_COVERAGE},
    "postgres_repository": {"coverage": $postgres_repository_coverage, "threshold": $MIN_POSTGRES_REPOSITORY_COVERAGE},
    "ai": {"coverage": $ai_coverage, "threshold": $MIN_AI_COVERAGE},
    "domain": {"coverage": $domain_coverage, "threshold": $MIN_DOMAIN_COVERAGE}
  },
  "test_count": $test_count,
  "test_count_threshold": $MIN_BACKEND_TEST_COUNT,
  "postgres_benchmark_count": $postgres_benchmark_count,
  "postgres_benchmark_count_threshold": $MIN_POSTGRES_BENCHMARK_COUNT,
  "rabbitmq_benchmark_count": $rabbitmq_benchmark_count,
  "rabbitmq_benchmark_count_threshold": $MIN_RABBITMQ_BENCHMARK_COUNT,
  "run_postgres_integration": "${RUN_POSTGRES_INTEGRATION:-0}"
}
EOF

cat "$COVER_SUMMARY"
