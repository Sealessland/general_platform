# 文档索引

这是仓库的文档入口，优先从这里定位信息，而不是在多个 agent 文件之间来回查找。

## 优先阅读

- 项目总览：`../README.md`
- 项目约束：`project-constraints.md`
- 架构边界：`architecture.md`
- 完成检查清单：`checklists/agent-native-completion.md`

## API 文档

- OpenAPI 契约：`api/openapi.yaml`
- 接口文档表：`api/endpoint-table.md`

## 工作流文档

- AI Native 开发工作流：`../AI_WORKFLOW.md`
- 新增或修改功能：`workflows/add-feature.md`
- 新增集成或适配器：`workflows/add-integration.md`
- Git worktree 协作：`workflows/git-worktree.md`
- 排查故障：`workflows/debug.md`
- 交付前验证：`workflows/validate.md`

## 风险与测试记录

- 测试策略：`testing/test-strategy.md`
- 端到端用例：`testing/e2e-cases.md`
- 性能基线：`testing/performance-baseline.md`
- 已确认风险归档：`testing/2026-06-05-risk-audit.md`
- 当前验证状态：`testing/2026-06-08-validation-status.md`

## 架构决策

- 使用 monorepo：`adr/0001-use-monorepo.md`
- 订单状态机：`adr/0002-order-state-machine.md`
- 库存锁策略：`adr/0003-inventory-lock-strategy.md`
- AI Provider 抽象：`adr/0004-ai-provider-abstraction.md`
- 服务边界与 RPC 使用场景：`adr/0005-service-boundaries-and-rpc.md`

## 关键行为

- 订单状态变更幂等：`architecture/order-action-idempotency.md`

## Agent 路由

- 仓库级说明：`../AGENTS.md`
- 本地 agent 注册：`../.agents/README.md`
- Codex 技能入口：`../.codex/skills/agent-native-shop/SKILL.md`
- 项目 Codex hook：`../.codex/config.toml`、`../.codex/hooks/redcart_project_hook.py`
- 本地状态板：`../BRANCH_STATUS.local.md`（自动生成，不提交）

面向 agent 的文件只负责路由，不重复维护业务事实；稳定事实统一落到 `docs/`。
