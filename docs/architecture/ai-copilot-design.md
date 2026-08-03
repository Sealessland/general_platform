# AI Copilot 设计

## Provider 契约

业务代码依赖统一的 `AIProvider` 接口，不直接依赖供应商 SDK。

当前要求覆盖的能力：

- 商品卖点生成
- 经营复盘生成

## 实现形态

- `MockProvider`：后端进程内当前实现的确定性规则适配器（`backend/internal/ai`），用于本地演示、测试和 CI；ai-service 侧对应实现为 `RuleBasedProvider`
- `OpenAIProvider`：规划中的真实线上适配器
- `QwenProvider`：规划中的备选适配器
- `LocalModelProvider`：规划中的本地推理适配器

## 当前 MVP 暴露的接口

- `POST /api/ai/product-selling-points`
- `POST /api/ai/business-review`
- `GET /api/ai/tasks/{id}`
- `POST /api/ai/a2ui`

当前后端会把请求记录为 AI 任务，并默认由进程内 `MockProvider`（`AI_PROVIDER=grpc` 时为 ai-service 的 `RuleBasedProvider`）返回可重复的规则生成草案结果，保证前端、测试和流程演示都可稳定运行。

### A2UI 界面生成

`POST /api/ai/a2ui` 接收 `surface_id`、`user_intent` 与可选 `context_json`，返回 `surface_id` 与 `a2ui_json`。`a2ui_json` 是 A2UI v0.9 的多行 NDJSON 指令流，逐行包含 `createSurface` / `updateComponents` / `updateDataModel` 指令；服务端在 `backend/internal/redcart/application/service_ai.go` 的 `GenerateA2UISurface` 中先做上下文富化（预算换算与相关笔记），再调用 `backend/internal/ai` 的 `AIProvider` 生成。前端 `frontend/src/app.ts` 的 `a2uiView` / `a2uiRenderPanel` 逐行解析 JSONL 并按指令装配组件树，支持文本、图片、列表、滑块与加购动作。

## 记录内容

AI 任务至少记录：

- 输入内容
- 输出内容
- 任务状态
- 错误信息
- 调用时间

## 安全边界

AI 输出只能作为建议，不能直接改动价格、库存、权限、订单状态或退款结算结果。
