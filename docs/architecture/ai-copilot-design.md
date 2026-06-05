# AI Copilot 设计

## Provider 契约

业务代码依赖统一的 `AIProvider` 接口，不直接依赖供应商 SDK。

当前要求覆盖的能力：

- 商品卖点生成
- 经营复盘生成

## 实现形态

- `MockAIProvider`：用于本地演示、测试和 CI
- `OpenAIProvider`：规划中的真实线上适配器
- `QwenProvider`：规划中的备选适配器
- `LocalModelProvider`：规划中的本地推理适配器

## 当前 MVP 暴露的接口

- `POST /api/ai/product-selling-points`
- `POST /api/ai/business-review`
- `GET /api/ai/tasks/{id}`

当前后端会把请求记录为 AI 任务，并返回可重复的 mock 草案结果，保证前端、测试和流程演示都可稳定运行。

## 记录内容

AI 任务至少记录：

- 输入内容
- 输出内容
- 任务状态
- 错误信息
- 调用时间

## 安全边界

AI 输出只能作为建议，不能直接改动价格、库存、权限、订单状态或退款结算结果。
