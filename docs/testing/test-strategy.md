# 测试策略

## 分层

- 领域层测试：订单状态机、金额规则、库存规则
- 应用层测试：下单、幂等重试、支付、取消、退款、库存恢复
- 集成测试：PostgreSQL 迁移、Redis 库存锁、事件链路
- 契约测试：OpenAPI 与 AI Provider 契约
- HTTP 测试：认证、购物车、结算、商家订单、AI 任务接口
- PostgreSQL-backed HTTP 测试：在 `RUN_POSTGRES_INTEGRATION=1` 时验证 Gin -> 应用层 -> PostgreSQL/GORM 的真实运行路径，包括主链路、并发库存、库存补偿、错误 method、越权和库存不足无副作用
- 前端测试：类型检查、lint、构建与主流程冒烟

## 质量门禁

- 后端：`go test ./...`
- 前端：`npm run typecheck`、`npm run lint`、`npm run build`
- AI 服务：Python 测试与提示词检查
- 仓库：`bash scripts/validate-workspace.sh`
- API：`bash scripts/check-openapi.sh`

## 性能基线口径

- 运行时性能基线只使用 PostgreSQL-backed benchmark。
- `BenchmarkHTTPNotes` 与 `BenchmarkHTTPOrderPreview` 使用内存仓储，只作为诊断数据，不进入 baseline。
- `BenchmarkHTTPPostgresOrderPreview` 使用真实 PostgreSQL 仓储，衡量读多查询路径的运行时基线。
- `BenchmarkHTTPPostgresCreateOrder` 使用真实 PostgreSQL 仓储，覆盖幂等下单、事务、库存条件更新、订单明细和库存锁写入。
- PostgreSQL benchmark 默认不在普通单元测试中执行；需要 `RUN_POSTGRES_INTEGRATION=1` 和 `POSTGRES_DSN`。

## 高风险场景

- 重复下单
- 库存不足
- 库存正好扣到 0
- 取消订单或退款后库存恢复不正确
- 非法状态流转
- GET 或错误 method 触发写操作
- 消费者访问商家接口、跨用户读取订单、跨商家读取 AI 任务
- 退款金额超过支付金额
- 优惠重复使用
- 高流量 SQL 缺索引
