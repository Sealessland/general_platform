# ADR 0008：Agent Copilot 的运行边界

- 状态：提议
- 日期：2026-08-02

## 为什么现在适合加 Agent

项目已经具备 Agent 需要的三类基础：JWT Actor 提供可信身份和角色，Application Service 提供稳定业务用例，`ai-service` 与 A2UI 提供模型和声明式界面。但现有“卖点生成、经营复盘、A2UI 生成”都是单次模型调用，没有工具选择、多步状态、停止条件、审批和可回放轨迹，因此不能描述成 Agent。

## 推荐的第一个 Agent

先做“商家经营诊断 Agent”，目标是回答：某个时间窗口内，哪个商品环节损失最大，下一步应该验证什么。

第一阶段只开放只读工具：

| 工具 | 复用能力 | 权限 |
|---|---|---|
| `get_dashboard_summary` | 商家经营汇总 | 当前 JWT 商家 |
| `get_conversion_funnel` | 浏览到支付漏斗 | 当前 JWT 商家 |
| `list_merchant_products` | 商家商品列表 | 当前 JWT 商家 |
| `get_product_detail` | 商品、SKU、库存快照 | 当前 JWT 商家 |
| `draft_business_review` | 现有 AI 经营复盘 | 只生成草案 |
| `render_action_plan` | 现有 A2UI | 只生成声明式 UI |

工具不接受模型传入的 `merchant_id`；它始终从 JWT Actor 取得作用域，防止提示注入造成跨商家访问。

## 放在哪里

```text
HTTP /api/agent/runs
        ↓
redcart/application/service_agent.go   身份、权限、预算、工具调度
        ↓                         ↓
internal/agent                    现有 Application 用例
中立 Planner/Tool/RunStore 契约          ↓
        ↓                    PostgreSQL / Redis
internal/agent/grpc
        ↓
ai-service（只做规划与生成，不直连业务库）
```

- `backend/internal/agent`：保存与电商无关的 `Planner`、`ToolCall`、`Run`、`Step`、停止原因等中立类型。
- `backend/internal/redcart/application/service_agent.go`：把工具名映射到现有业务用例，并在每次调用前重新校验 Actor 和参数。
- `backend/internal/agent/grpc`：调用 `ai-service` 的 Planner，不把 gRPC 类型泄漏给应用层。
- `ai-service`：给定目标、可用工具 schema 和前序结果，返回下一步工具调用或最终回答；它无数据库凭证，也不能绕过 Go 后端执行写操作。
- A2UI：只渲染 Agent 的结果和审批卡片，不负责决定业务操作。

## 状态与多实例执行

Agent Run 是需要恢复和审计的业务状态，应保存在 PostgreSQL，而不是 prompt 或进程内存：

- `agent_runs`：owner、merchant、goal、status、step_budget、deadline、created_at。
- `agent_steps`：step_no、tool、脱敏输入、输出摘要、耗时、状态、错误。
- `agent_approvals`：待审批动作、审批人、决定和时间。

两个 API 实例通过数据库 worker 领取待执行 Run，使用 `FOR UPDATE SKIP LOCKED` 保证同一 Run 只有一个执行者。第一版不为了排队而虚构 Kafka Consumer；当真实异步 worker 独立部署后，再把 `agent.run.requested` 接到 Kafka，并同时实现 Inbox 去重。

Redis 只适合短期租约、速率限制和取消信号，不作为 Run 的持久化真相。

## 写工具与审批

第二阶段才能增加写工具，并把“预览”和“执行”拆开：

```text
Agent 提议 update_product_selling_points
  → preview_product_update（无副作用）
  → status=waiting_approval
  → 商家显式 approve
  → update_product（携带 approval_id + idempotency_key）
  → verify_product_update
```

支付、退款审批、库存调整、删除商品不进入第一批 Agent 工具。未来开放时必须单独设计金额/库存上限、二次确认、幂等和审计，不允许模型直接调用 Repository。

## Agent Loop 与停止条件

- 单 Run 最多 8 个步骤、30 秒墙钟时间、固定模型 token 预算。
- 单工具默认 2 秒超时；同一错误最多重试 1 次。
- Planner 只能选择注册表内工具；未知工具、schema 不合法或越权立即终止。
- 终止状态包括 `completed`、`waiting_approval`、`budget_exhausted`、`denied`、`failed`、`cancelled`。
- 每一步都携带 `agent_run_id` 和 `correlation_id`，日志不记录 JWT、密码、完整 prompt 或顾客隐私。

## 公共接口草案

- `POST /api/agent/runs`：提交目标，返回 Run。
- `GET /api/agent/runs/{id}`：查询状态、步骤摘要和最终结果。
- `POST /api/agent/runs/{id}/cancel`：取消未完成 Run。
- `POST /api/agent/runs/{id}/approvals/{approval_id}`：批准或拒绝待执行写动作。

第一版可以同步执行只读 Run，但仍然持久化 Run/Step；不要先返回假的异步 task ID，再在请求内悄悄跑完。

## 验证与 Evals

必须覆盖：

- 正常诊断能选择正确工具并在预算内停止。
- 工具超时、模型返回未知工具、参数不合法时安全终止。
- 消费者不能运行商家 Agent，商家不能读取其他商家的 Run。
- prompt 注入要求传入其他 `merchant_id` 时仍只能读取当前 Actor 数据。
- 写工具未审批不得执行；重复审批和重复执行保持幂等。
- 固定经营数据 fixture 的 tool trace 可回放，并检查结论引用的指标确实来自工具结果。

## 非目标

- 不把普通 CRUD 包装成几十个“智能体”。
- 不让模型直接生成 SQL、访问 Redis/Kafka 或持有数据库凭证。
- 不在第一版引入多 Agent 委派、长期用户记忆、向量数据库或自动下单。
- 不把 A2UI 动作事件当成已经完成的业务写入。
