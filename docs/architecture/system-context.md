# 系统上下文

RedCart Copilot 面向四类直接可见的使用面：

- 消费者购物链路：笔记、商品、购物车、结算、订单、退款
- 商家经营链路：商品、SKU、库存、订单、看板
- AI Copilot 链路：卖点生成、经营复盘、任务记录
- 工程协作链路：任务、PR、CI、测试、迁移、ADR、AI 工作流记录

## 运行组件

- `backend`：Go API 服务与业务模块
- `frontend`：消费者与商家共用的前端演示应用
- `ai-service`：提示词与 AI Provider 占位实现
- `postgresql`：目标中的业务数据源
- `redis`：目标中的购物车、库存预扣、幂等与热点缓存
- `rabbitmq`：规划中的订单、库存、分析事件总线

## 当前 MVP 的运行边界

当前可执行 MVP 明确保持了分层边界：

- 产品接口层：`backend/cmd/api` 和 `frontend/`
- 运行编排层：`backend/internal/redcart/application`
- 领域能力层：`backend/internal/order/domain` 与 `backend/internal/redcart/domain`
- 集成适配层：内存仓储 `backend/internal/redcart/infrastructure/memory` 与 Mock AI Provider `backend/internal/ai`

这样做的目的，是让主链路先可运行、可演示、可测试；PostgreSQL、Redis、RabbitMQ 仍然作为下一阶段适配目标存在，而不是混进当前领域逻辑里。

## 核心数据流

1. 消费者浏览笔记与商品
2. 行为事件被记录
3. 消费者加购 SKU
4. 结算预览校验库存与金额
5. 下单时携带幂等键并创建库存锁
6. 支付成功后订单进入已支付状态
7. 订单与行为事件汇总到商家看板
8. AI Copilot 基于商品与经营数据返回草案建议
