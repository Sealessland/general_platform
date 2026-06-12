# 2026-06-08 验证状态

本记录归档 HTTP 测试按主题拆分后的本地验证状态，以及项目专用 Codex hook 的交付前检查口径。

## 当前状态

- HTTP 层测试已从单一 `server_test.go` 拆分为基础路由、购物车、订单、商家、AI、PostgreSQL 集成、benchmark 和公共 helper 文件。
- 应用层追加测试已从单一 `service_additional_test.go` 拆分到 auth、checkout、order、merchant dashboard 和 AI 等主题文件。
- 拆分后原有 `Test*` 与 `Benchmark*` 入口未丢失。
- 本地完整 CI 门禁已通过，包括真实 PostgreSQL 仓储与 PostgreSQL-backed HTTP 集成测试。

## 验证证据

```bash
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk bash ci/scripts/frontend-ci.sh
rtk bash ci/scripts/ai-service-ci.sh
rtk bash ci/scripts/security-ci.sh
rtk bash ci/scripts/check-openapi.sh
rtk bash ci/scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
rtk docker build -t redcart-backend:ci backend
rtk docker build -t redcart-frontend:ci frontend
rtk docker build -t redcart-ai-service:ci ai-service
```

后端质量门禁结果：

- 总覆盖率：`80.9%`
- 应用层覆盖率：`85.0%`
- HTTP 层覆盖率：`67.3%`
- 内存仓储覆盖率：`94.2%`
- PostgreSQL 仓储覆盖率：`80.3%`
- AI 包覆盖率：`100.0%`
- 领域模型覆盖率：`100.0%`
- 后端测试数量：`64`
- 内存诊断 benchmark 数量：`2`
- PostgreSQL benchmark 数量：`2`

## 项目专用 Codex Hook

项目本地 Codex hook 配置位于 `.codex/config.toml`，脚本位于 `.codex/hooks/redcart_project_hook.py`。

当前 hook 行为：

- `PreToolUse`：拦截 Codex Bash 调用，要求本仓库 shell 命令使用 `rtk` 前缀，并阻止明显高风险的破坏性命令。
- `PostToolUse`：在相关 Git/worktree/stash 操作后刷新主工作区根目录的 `BRANCH_STATUS.local.md`，同步 worktree 状态与 AI 更改大纲。
- `Stop`：在 Codex 准备结束交付时运行 quick handoff gate，检查空白、OpenAPI、workspace 结构、测试入口未丢失，并按变更范围运行相关测试。

手动验证命令：

```bash
rtk python3 .codex/hooks/redcart_project_hook.py --self-test
rtk python3 scripts/update-branch-status.py
rtk python3 .codex/hooks/redcart_project_hook.py --mode quick
rtk python3 .codex/hooks/redcart_project_hook.py --mode full
```

## 剩余边界

- `Stop` hook 默认使用 quick 模式，适合交付前兜底；完整 PostgreSQL 集成、benchmark、前端、AI service 和安全门禁仍应在高风险改动后手动运行 full 模式或对应 CI 脚本。
- `BRANCH_STATUS.local.md` 是本地状态板，不提交进仓库；其 AI 更改大纲会在本地 Codex 不可用或超时时回退到 Git 事实摘要。
- 项目本地 hook 需要在 Codex 的 `/hooks` 面板中被信任后才会自动运行；脚本修改后需要重新信任。
- 前端仍未引入浏览器级 E2E 自动化；当前前端门禁是源码守卫、类型检查、lint 和构建。
