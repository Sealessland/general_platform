# 项目约束

这份文档定义 RedCart Copilot 当前阶段的硬约束。所有功能实现、结构调整、提交流程和发布动作，都应以这里为准。

## 1. 项目目标

- 当前阶段目标是 **内容电商 MVP**。
- 优先保证主链路可运行、可演示、可解释，不追求一次性做成完整生产系统。
- 所有新增工作都应优先服务这两类主流程：
  - 消费者：笔记流 -> 商品详情 -> 购物车 -> 结算 -> 下单 -> 支付 -> 订单流转
  - 商家：商品管理 -> SKU 管理 -> 订单履约 -> 经营看板 -> AI 辅助

## 2. 架构边界

- 公共入口必须清晰，优先从这些位置就能看懂系统：
  - `backend/cmd/api`
  - `frontend/`
  - `docs/api/openapi.yaml`
  - `docker-compose.yml`
- 领域规则放在领域层，流程编排放在应用层，环境相关逻辑放在基础设施层。
- 不允许把数据库、中间件或供应商细节偷偷塞进领域层。

## 3. 运行时约束

- 当前 MVP 运行环境以 **Docker Compose** 为准。
- 运行时数据库为 **PostgreSQL**，运行时缓存/会话源为 **Redis**。
- 后端启动时必须显式提供 `POSTGRES_DSN` 与 `REDIS_ADDR`。
- 本地推荐入口：

```bash
bash scripts/local-dev.sh
```

- `docker-compose.yml` 是当前 MVP 的运行事实源。

## 4. Git 与提交约束

- 默认从 `main` 或当前稳定开发分支拉功能分支，并为每条任务线创建独立 `git worktree`。
- 不在长期开发分支上直接堆中间件试验、性能 spike、QPS 对照或高风险排查；这类工作先开独立实验分支，或至少用带说明的 `git stash` 保留现场。
- 试验性改动只有在验证结论稳定、风险可解释、且通过对应门禁后才允许提交；如果基准、QPS、正确性或运行门禁出现异常，不提交到当前主开发分支。
- 提交前先用暂存差异复核本次提交范围，避免把无关试验、半成品或临时调试代码混进同一个 commit。
- 提交信息使用 Conventional Commits。
- 数据结构变化必须同步：
  - `backend/migrations/`
  - `docs/api/openapi.yaml`
  - 必要的架构文档或 ADR
- AI 参与的设计与实现修正，必须记录到 `AI_WORKFLOW.md` 或 PR 描述中。

## 5. CI/CD 约束

- GitHub Actions 入口保留在 `.github/workflows/`
- CI/CD 说明和脚本统一放在 `ci/`
- `scripts/` 只保留开发者兼容入口，不承载复杂 CI 逻辑
- 相关说明见：

```text
ci/README.md
```

## 6. 交付约束

- 对外文档统一使用中文
- 文档中不暴露本地代理规则
- 交付前至少通过：

```bash
bash scripts/validate-workspace.sh
bash scripts/check-openapi.sh
```

- 只要结构、边界或工作流变化，就必须同步更新文档
