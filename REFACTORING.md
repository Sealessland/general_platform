# 重构计划

## 当前技术债

| 编号 | 模块 | 问题 | 影响 | 优先级 |
|---|---|---|---|---|
| TD-001 | order | 下单流程目前仍集中在单个服务中 | 后续接入真实基础设施时拆分成本偏高 | P0 |
| TD-002 | inventory | Redis Lua 预扣库存仍停留在设计层 | 并发防超卖还没有真实适配器实现 | P0 |
| TD-003 | ai | AI Provider 仍以 Mock 为主 | 真实 Provider 的失败、成本和限流策略未覆盖 | P1 |
| TD-004 | frontend | 当前前端是静态演示应用 | 复杂交互和更细粒度状态管理仍有演进空间 | P1 |

## 重构约束

1. 不改变外部 API 行为，除非同步更新 OpenAPI 和测试。
2. 行为保持不变的重构，优先先补测试再动实现。
3. 每个 PR 只聚焦一个重构目标。
4. 架构边界变化必须同步更新 ADR 或架构文档。
5. 已发布迁移文件不原地修改，新增迁移承接变更。

## 计划中的重构

### RF-001 拆分下单流程

目标拆分：

- `OrderValidator`
- `InventoryLocker`
- `OrderFactory`
- `OrderEventPublisher`

完成证据：

- 覆盖重复提交、库存不足、事件记录等测试
- `go test ./...` 通过

已完成切片：

- 抽出订单草稿和库存锁构建逻辑，降低 `CreateOrder` 主流程复杂度。
- 补充订单创建副作用测试，覆盖创建事件、行为事件、库存锁和 locked stock 变化。

### RF-002 落地真实库存适配层

目标拆分：

- 内存实现保留给本地演示和单元测试
- Redis Lua 负责预扣库存
- PostgreSQL 负责最终库存和锁记录

完成证据：

- 并发下单测试可重复运行
- 库存锁释放与补偿路径可验证

### RF-003 AI Provider 注册化

目标拆分：

- `AIProvider` 契约
- `MockProvider`
- `OpenAIProvider`
- `QwenProvider`
- 配置驱动的 Provider 选择

完成证据：

- 契约测试覆盖 Mock Provider
- 真实 Provider 可在 CI 中关闭
