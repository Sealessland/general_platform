# CI/CD 目录说明

`ci/` 是仓库中专门承载 CI/CD 规则、脚本和说明的目录。

## 目录职责

- `.github/workflows/`
  - 只负责 GitHub Actions 的触发和编排
  - 不直接承载复杂校验逻辑
- `ci/scripts/`
  - 承载各类质量检查脚本
  - 本地与 GitHub Actions 应优先共用这些脚本
- `scripts/`
  - 保留开发者兼容入口
  - 可以转发到 `ci/scripts/*`

## 当前脚本

- `ci/scripts/backend-ci.sh`：后端格式、静态检查、单元测试、集成测试、覆盖率、迁移和构建检查
- `ci/scripts/backend-test-metrics.sh`：后端测试指标、覆盖率阈值和 benchmark 数量门禁
- `ci/scripts/frontend-ci.sh`：前端类型检查、lint、单元测试和构建
- `ci/scripts/ai-service-ci.sh`：AI service 的 Python 检查、单元测试和 prompt 检查
- `ci/scripts/security-ci.sh`：安全与基线检查入口
- `ci/scripts/check-openapi.sh`：OpenAPI 契约快速检查
- `ci/scripts/scan-secrets.sh`：密钥泄露扫描
- `ci/scripts/validate-workspace.sh`：仓库结构验证

## 设计原则

- 所有 CI 步骤应能在本地复现
- YAML 只负责编排，不做复杂逻辑
- 先有脚本，再由 workflow 调用
- 项目约束优先体现在脚本和文档里，而不是只靠口头约定

## Workflow 触发策略

- `.github/workflows/ci.yml` 是 PR 和 `main` push 的统一门禁入口
- `backend-test.yml`、`frontend-test.yml`、`ai-service-test.yml`、`security-test.yml` 和 `docker-build.yml` 只保留 `workflow_call` 与 `workflow_dispatch`
- 子 workflow 可被顶层 CI 复用，也可手动单独运行；不再直接监听 PR 或 `main` push，避免同一事件重复执行
- 后端 CI 需要同时启动当前运行时依赖 PostgreSQL 与 Redis；两者都是 MVP 运行前置服务
- Dependabot 只配置真实存在的依赖面：Go module、npm、Dockerfile 和 GitHub Actions
- Pyroscope profile 查询不进入 GitHub Actions 门禁；CI 只覆盖 profiling 配置单测和常规后端门禁，profile 数据保留为本地人工/界面复核

## QPS 输出

后端 CI 会执行运行时 HTTP 基准测试，并在 `ci/artifacts/` 下产出：

- `backend-postgres-http-benchmark.txt`
- `backend-postgres-http-qps.txt`

`backend-postgres-http-benchmark.txt` 和 `backend-postgres-http-qps.txt` 只有在 `RUN_POSTGRES_INTEGRATION=1` 时执行，使用真实 PostgreSQL 仓储，覆盖 Gin -> 应用层 -> GORM/PostgreSQL 的读写路径。它们是评估 PostgreSQL/GORM 迁移后的运行时 QPS 基线。

脚本也会保留 `backend-benchmark.txt` 和 `backend-qps.txt` 作为内存仓储诊断产物，用于观察 Gin HTTP 路由、JSON 编解码和应用层轻量路径；它们不进入运行时性能 baseline。

所有 QPS 文件都会把 `ns/op` 换算成 QPS，便于在 GitHub Actions 或本地 CI 产物中直接查看。

## 后端质量指标

后端 CI 会在测试和 benchmark 通过后运行 `ci/scripts/backend-test-metrics.sh`，并在 `ci/artifacts/` 下产出：

- `backend-test.txt`：`go test ./... -coverprofile=coverage.out` 原始输出
- `backend-test-list.txt`：后端 `Test*` 清单，用于追踪测试规模
- `backend-coverage-functions.txt`：`go tool cover -func` 函数级覆盖率
- `backend-coverage-summary.txt`：人类可读的质量指标摘要
- `backend-test-metrics.json`：机器可读的覆盖率、测试数量和 benchmark 数量指标

当前门禁阈值使用脚本内默认值，也可以通过环境变量覆盖：

- `MIN_TOTAL_COVERAGE=65.0`
- `MIN_APPLICATION_COVERAGE=80.0`
- `MIN_HTTPAPI_COVERAGE=60.0`
- `MIN_MEMORY_COVERAGE=90.0`
- `MIN_POSTGRES_REPOSITORY_COVERAGE=15.0`，当 `RUN_POSTGRES_INTEGRATION=1` 时默认提高到 `75.0`
- `MIN_AI_COVERAGE=95.0`
- `MIN_DOMAIN_COVERAGE=95.0`
- `MIN_BACKEND_TEST_COUNT=55`
- `MIN_BACKEND_BENCHMARK_COUNT=2`
- `MIN_POSTGRES_BENCHMARK_COUNT=2`

阈值按当前 MVP 的可稳定通过水平设置，目标是阻止测试规模和关键包覆盖率回退；后续应随功能稳定逐步提高阈值。
