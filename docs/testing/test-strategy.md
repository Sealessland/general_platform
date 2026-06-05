# 测试策略

## 分层

- 领域层测试：订单状态机、金额规则、库存规则
- 应用层测试：下单、幂等重试、支付、取消、退款、库存恢复
- 集成测试：PostgreSQL 迁移、Redis 库存锁、事件链路
- 契约测试：OpenAPI 与 AI Provider 契约
- HTTP 测试：认证、购物车、结算、商家订单、AI 任务接口
- 前端测试：类型检查、lint、构建与主流程冒烟

## 质量门禁

- 后端：`go test ./...`
- 前端：`npm run typecheck`、`npm run lint`、`npm run build`
- AI 服务：Python 测试与提示词检查
- 仓库：`bash scripts/validate-workspace.sh`
- API：`bash scripts/check-openapi.sh`

## 高风险场景

- 重复下单
- 库存不足
- 库存正好扣到 0
- 取消订单或退款后库存恢复不正确
- 非法状态流转
- 退款金额超过支付金额
- 优惠重复使用
- 高流量 SQL 缺索引
