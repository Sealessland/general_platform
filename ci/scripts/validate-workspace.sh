#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

required_files=(
  "README.md"
  "AGENTS.md"
  ".gitignore"
  "CHANGELOG.md"
  "CONTRIBUTING.md"
  "AI_WORKFLOW.md"
  "REFACTORING.md"
  "docker-compose.yml"
  ".env.example"
  "docs/index.md"
  "docs/architecture.md"
  "docs/project-constraints.md"
  "docs/api/openapi.yaml"
  "docs/prd/001-content-to-order.md"
  "docs/prd/002-merchant-dashboard.md"
  "docs/prd/003-ai-copilot.md"
  "docs/architecture/system-context.md"
  "docs/architecture/order-state-machine.md"
  "docs/architecture/inventory-design.md"
  "docs/architecture/ai-copilot-design.md"
  "docs/adr/0001-use-monorepo.md"
  "docs/adr/0002-order-state-machine.md"
  "docs/adr/0003-inventory-lock-strategy.md"
  "docs/adr/0004-ai-provider-abstraction.md"
  "docs/testing/test-strategy.md"
  "docs/testing/e2e-cases.md"
  "docs/workflows/add-feature.md"
  "docs/workflows/add-integration.md"
  "docs/workflows/debug.md"
  "docs/workflows/validate.md"
  "docs/checklists/agent-native-completion.md"
  "ci/README.md"
  "ci/scripts/check-openapi.sh"
  "ci/scripts/scan-secrets.sh"
  "ci/scripts/validate-workspace.sh"
  "ci/scripts/backend-ci.sh"
  "ci/scripts/backend-test-metrics.sh"
  "ci/scripts/frontend-ci.sh"
  "ci/scripts/ai-service-ci.sh"
  "ci/scripts/security-ci.sh"
  ".github/workflows/ci.yml"
  ".github/workflows/backend-test.yml"
  ".github/workflows/frontend-test.yml"
  ".github/workflows/ai-service-test.yml"
  ".github/workflows/docker-build.yml"
  ".github/workflows/release.yml"
  ".github/workflows/security-test.yml"
  ".github/ISSUE_TEMPLATE/feature_request.md"
  ".github/ISSUE_TEMPLATE/bug_report.md"
  ".github/ISSUE_TEMPLATE/tech_debt.md"
  ".github/ISSUE_TEMPLATE/refactor_task.md"
  ".github/PULL_REQUEST_TEMPLATE.md"
  ".github/dependabot.yml"
  ".agents/README.md"
  ".codex/skills/agent-native-shop/SKILL.md"
  "backend/go.mod"
  "backend/cmd/api/main.go"
  "backend/internal/order/domain/order_status.go"
  "backend/internal/order/domain/order_status_test.go"
  "backend/internal/ai/provider.go"
  "backend/migrations/0001_init_schema.sql"
  "frontend/package.json"
  "frontend/src/app.ts"
  "ai-service/app/provider.py"
  "ai-service/app/check_prompts.py"
  "scripts/check-openapi.sh"
  "scripts/scan-secrets.sh"
  "scripts/validate-workspace.sh"
)

missing=0
for path in "${required_files[@]}"; do
  if [[ ! -f "$path" ]]; then
    printf 'missing required file: %s\n' "$path" >&2
    missing=1
  fi
done

if [[ "$missing" -ne 0 ]]; then
  exit 1
fi

grep -q "依赖方向" docs/architecture.md
grep -q "RedCart Copilot" README.md
grep -q "项目约束" docs/project-constraints.md
grep -q "CI/CD 目录说明" ci/README.md
grep -q "AI Native 开发工作流" AI_WORKFLOW.md
grep -q "重构计划" REFACTORING.md
grep -q "Conventional Commits" CONTRIBUTING.md
grep -q "订单状态机" docs/architecture/order-state-machine.md
grep -q "Idempotency-Key" docs/api/openapi.yaml
grep -q "idempotency_key" backend/migrations/0001_init_schema.sql
grep -q "AIProvider interface" backend/internal/ai/provider.go
grep -q "完成证据" docs/workflows/add-feature.md
grep -q "完成证据" docs/workflows/add-integration.md
grep -q "完成证据" docs/workflows/debug.md
grep -q "CHANGELOG.md" docs/workflows/validate.md
grep -q "bash scripts/validate-workspace.sh" README.md
grep -q "docs/index.md" AGENTS.md

bash ci/scripts/scan-secrets.sh

printf 'redcart copilot portfolio validation passed\n'
