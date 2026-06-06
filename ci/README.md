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

## QPS 输出

后端 CI 会执行运行时 HTTP 基准测试，并在 `ci/artifacts/` 下产出：

- `backend-postgres-http-benchmark.txt`
- `backend-postgres-http-qps.txt`

`backend-postgres-http-benchmark.txt` 和 `backend-postgres-http-qps.txt` 只有在 `RUN_POSTGRES_INTEGRATION=1` 时执行，使用真实 PostgreSQL 仓储，覆盖 Gin -> 应用层 -> GORM/PostgreSQL 的读写路径。它们是评估 PostgreSQL/GORM 迁移后的运行时 QPS 基线。

脚本也会保留 `backend-benchmark.txt` 和 `backend-qps.txt` 作为内存仓储诊断产物，用于观察 Gin HTTP 路由、JSON 编解码和应用层轻量路径；它们不进入运行时性能 baseline。

所有 QPS 文件都会把 `ns/op` 换算成 QPS，便于在 GitHub Actions 或本地 CI 产物中直接查看。
