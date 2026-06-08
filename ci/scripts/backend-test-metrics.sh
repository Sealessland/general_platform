#!/usr/bin/env bash
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

MIN_TOTAL_COVERAGE="${MIN_TOTAL_COVERAGE:-65.0}"
MIN_APPLICATION_COVERAGE="${MIN_APPLICATION_COVERAGE:-80.0}"
MIN_HTTPAPI_COVERAGE="${MIN_HTTPAPI_COVERAGE:-60.0}"
MIN_MEMORY_COVERAGE="${MIN_MEMORY_COVERAGE:-90.0}"
MIN_AI_COVERAGE="${MIN_AI_COVERAGE:-95.0}"
MIN_DOMAIN_COVERAGE="${MIN_DOMAIN_COVERAGE:-95.0}"
MIN_BACKEND_TEST_COUNT="${MIN_BACKEND_TEST_COUNT:-55}"
MIN_BACKEND_BENCHMARK_COUNT="${MIN_BACKEND_BENCHMARK_COUNT:-2}"
MIN_POSTGRES_BENCHMARK_COUNT="${MIN_POSTGRES_BENCHMARK_COUNT:-2}"

if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  MIN_POSTGRES_REPOSITORY_COVERAGE="${MIN_POSTGRES_REPOSITORY_COVERAGE:-75.0}"
else
  MIN_POSTGRES_REPOSITORY_COVERAGE="${MIN_POSTGRES_REPOSITORY_COVERAGE:-15.0}"
fi

cd "$BACKEND_DIR"

if [[ ! -f "$COVERPROFILE" ]]; then
  printf 'missing coverage profile: %s\n' "$COVERPROFILE" >&2
  exit 1
fi
if [[ ! -f "$BACKEND_TEST_LOG" ]]; then
  printf 'missing backend test log: %s\n' "$BACKEND_TEST_LOG" >&2
  exit 1
fi

go tool cover -func="$COVERPROFILE" | tee "$COVER_FUNCTIONS" >/dev/null

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

total_coverage="$(awk '/^total:/ {value=$3; gsub("%", "", value); print value}' "$COVER_FUNCTIONS")"
application_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/application")"
httpapi_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi")"
memory_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory")"
postgres_repository_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres")"
ai_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/ai")"
domain_coverage="$(coverage_for_package "github.com/example/redcart-copilot/backend/internal/redcart/domain")"

test_count="$(go test ./... -list '^Test' | tee "$TEST_LIST" | awk '/^Test/ {count++} END {print count + 0}')"
benchmark_count="$(awk '/^BenchmarkHTTP/ {count++} END {print count + 0}' "$ARTIFACT_DIR/backend-benchmark.txt" 2>/dev/null || printf '0')"
postgres_benchmark_count="$(awk '/^BenchmarkHTTPPostgres/ {count++} END {print count + 0}' "$ARTIFACT_DIR/backend-postgres-http-benchmark.txt" 2>/dev/null || printf '0')"

require_number_at_least "total coverage" "$total_coverage" "$MIN_TOTAL_COVERAGE"
require_number_at_least "application package coverage" "$application_coverage" "$MIN_APPLICATION_COVERAGE"
require_number_at_least "httpapi package coverage" "$httpapi_coverage" "$MIN_HTTPAPI_COVERAGE"
require_number_at_least "memory repository package coverage" "$memory_coverage" "$MIN_MEMORY_COVERAGE"
require_number_at_least "postgres repository package coverage" "$postgres_repository_coverage" "$MIN_POSTGRES_REPOSITORY_COVERAGE"
require_number_at_least "ai package coverage" "$ai_coverage" "$MIN_AI_COVERAGE"
require_number_at_least "domain package coverage" "$domain_coverage" "$MIN_DOMAIN_COVERAGE"
require_number_at_least "backend test count" "$test_count" "$MIN_BACKEND_TEST_COUNT"
require_number_at_least "backend benchmark count" "$benchmark_count" "$MIN_BACKEND_BENCHMARK_COUNT"
if [[ "${RUN_POSTGRES_INTEGRATION:-0}" == "1" ]]; then
  require_number_at_least "postgres benchmark count" "$postgres_benchmark_count" "$MIN_POSTGRES_BENCHMARK_COUNT"
fi

cat >"$COVER_SUMMARY" <<EOF
backend quality metrics
total_coverage=$total_coverage threshold=$MIN_TOTAL_COVERAGE
application_coverage=$application_coverage threshold=$MIN_APPLICATION_COVERAGE
httpapi_coverage=$httpapi_coverage threshold=$MIN_HTTPAPI_COVERAGE
memory_coverage=$memory_coverage threshold=$MIN_MEMORY_COVERAGE
postgres_repository_coverage=$postgres_repository_coverage threshold=$MIN_POSTGRES_REPOSITORY_COVERAGE
ai_coverage=$ai_coverage threshold=$MIN_AI_COVERAGE
domain_coverage=$domain_coverage threshold=$MIN_DOMAIN_COVERAGE
test_count=$test_count threshold=$MIN_BACKEND_TEST_COUNT
benchmark_count=$benchmark_count threshold=$MIN_BACKEND_BENCHMARK_COUNT
postgres_benchmark_count=$postgres_benchmark_count threshold=$MIN_POSTGRES_BENCHMARK_COUNT run_postgres_integration=${RUN_POSTGRES_INTEGRATION:-0}
EOF

cat >"$METRICS_JSON" <<EOF
{
  "total_coverage": $total_coverage,
  "coverage_threshold": $MIN_TOTAL_COVERAGE,
  "packages": {
    "application": {"coverage": $application_coverage, "threshold": $MIN_APPLICATION_COVERAGE},
    "httpapi": {"coverage": $httpapi_coverage, "threshold": $MIN_HTTPAPI_COVERAGE},
    "memory": {"coverage": $memory_coverage, "threshold": $MIN_MEMORY_COVERAGE},
    "postgres_repository": {"coverage": $postgres_repository_coverage, "threshold": $MIN_POSTGRES_REPOSITORY_COVERAGE},
    "ai": {"coverage": $ai_coverage, "threshold": $MIN_AI_COVERAGE},
    "domain": {"coverage": $domain_coverage, "threshold": $MIN_DOMAIN_COVERAGE}
  },
  "test_count": $test_count,
  "test_count_threshold": $MIN_BACKEND_TEST_COUNT,
  "benchmark_count": $benchmark_count,
  "benchmark_count_threshold": $MIN_BACKEND_BENCHMARK_COUNT,
  "postgres_benchmark_count": $postgres_benchmark_count,
  "postgres_benchmark_count_threshold": $MIN_POSTGRES_BENCHMARK_COUNT,
  "run_postgres_integration": "${RUN_POSTGRES_INTEGRATION:-0}"
}
EOF

cat "$COVER_SUMMARY"
