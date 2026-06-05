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
