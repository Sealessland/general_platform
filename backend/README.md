# Backend 阅读指南

这份后端按“先业务、后中间件”的顺序组织。第一次阅读不用从 `main.go` 一路点进所有实现，先记住下面五层即可。

```text
cmd/api                         启动：创建依赖并启动 HTTP / 后台任务
  ↓
internal/redcart/interfaces     接口：路由、认证、参数、响应、中间件
  ↓
internal/redcart/application    用例：下单、支付、退款等流程编排
  ↓
internal/redcart/domain         规则：订单、商品、用户等业务模型
  ↓
internal/redcart/infrastructure 适配：PostgreSQL、JWT 与 Redis

application → internal/event（中立事件契约）← event/kafka（Kafka 适配）
                                      ↑
                                event/outbox（发布编排）
```

箭头表示“可以依赖”。`domain` 不知道 Gin、GORM、Redis、Kafka；`application` 不知道 HTTP 和 Kafka SDK。

## 建议阅读顺序

1. `cmd/api/main.go`：看程序创建了哪些依赖。
2. `internal/redcart/interfaces/httpapi/server.go`：看公开路由。
3. `internal/redcart/application/service_order.go`：看一个完整下单用例。
4. `internal/redcart/domain/models.go`：看业务数据和不变量。
5. `internal/redcart/infrastructure/postgres/order_repository.go`：看业务如何落库。
6. `internal/redcart/infrastructure/auth/jwt_manager.go`：看 JWT 如何签发，以及 Redis 如何协调跨实例刷新和登出。
7. `internal/event/outbox/publisher.go` 和 `internal/event/kafka/publisher.go`：最后看异步发布。

## 一次下单经过哪些文件

```text
POST /api/orders
  → interfaces/httpapi/handlers_orders.go       解析请求、认证、返回 HTTP
  → application/service_order.go                编排幂等、金额、库存和订单
  → infrastructure/postgres/order_repository.go 在事务中写订单和库存锁
  → PostgreSQL outbox                            同事务记录待发布事件
  → event/outbox/publisher.go                    后台领取事件
  → event/kafka/publisher.go                     等待 Kafka broker ack
```

如果下单规则变化，先改 application/domain；如果换数据库或 broker，只改 infrastructure/event adapter；如果响应格式变化，只改 interfaces。

## JWT 与 Redis 怎么分工

认证的中立接口是 `application.TokenManager`，生产实现位于 `infrastructure/auth`：

- Access JWT 本地校验签名和标准 claims。
- Redis 只保存一次性 Refresh 会话和 Access `jti` 撤销标记。
- Refresh 轮换由 Lua 原子完成，两个实例同时刷新时只有一个成功。
- Repository 只管理业务数据，不参与 token 解析。

Redis 的其他职责放在 `internal/redcart/infrastructure/redis`：

- `catalog_cache_repository.go`：商品与 SKU 热读缓存。
- `rate_limiter.go`：Lua 原子令牌桶。

HTTP 限流策略放在 `interfaces/httpapi/middleware_rate_limit.go`。认证写和 AI 写在 Redis 故障时拒绝请求，公开商品/笔记读取则放行；这是接口层的可用性取舍，不属于 Redis 适配器本身。

## 本地多实例拓扑

```text
浏览器 → Nginx :18080
             ├→ backend :18081
             └→ backend-replica :18082
                         ↓
             PostgreSQL / Redis / Kafka
```

两个 API 实例运行完全相同的代码。Nginx 响应头 `X-RedCart-Upstream` 会显示本次请求落到哪个实例；跨实例 JWT、迁移锁、限流和 Outbox 的边界见 `../docs/adr/0007-jwt-and-multi-instance-runtime.md`。

启动 Compose 后，可以直接验证“实例 A 登录、实例 B 鉴权并登出、实例 A 立即拒绝旧 token”：

```bash
rtk bash ../scripts/verify-distributed-auth.sh
```

## Kafka 的最小模型

- 只有一个物理 topic：`redcart.events`。
- JSON 中的 `event_topic` 表示 `order.created` 等业务分类。
- `correlation_id` 是分区 key；没有时使用 `event_id`。
- Kafka ack 后才标记 outbox 已发布。
- 崩溃窗口仍可能产生重复消息，所以未来消费者必须按 `event_id` 持久化去重。

更完整的边界与限制见 `../docs/adr/0006-message-queue-and-event-driven.md`。

## Agent 应该放哪里

现有 A2UI 是 Agent 的展示层，不是工具执行层。后续 Agent 的中立运行契约放在 `internal/agent`，电商工具调度放在 `redcart/application/service_agent.go`，模型 Planner 适配器放在 `internal/agent/grpc`；AI service 不直接访问 PostgreSQL/Redis。

第一个建议实现“商家经营诊断 Agent”：先只读经营看板、漏斗和商品，再输出复盘与 A2UI 行动计划。写商品前必须先生成预览并等待商家审批。完整目录、状态、工具与安全边界见 `../docs/adr/0008-agent-copilot-runtime-boundary.md`。

## 新增功能放哪里

| 你要做的事 | 放置位置 |
|---|---|
| 新增 HTTP 路由或响应 | `interfaces/httpapi` |
| 编排一个业务流程 | `application` |
| 新增可复用业务规则 | `domain` |
| 新增 SQL/Redis 行为 | `infrastructure` |
| 修改 JWT claims 或刷新策略 | `infrastructure/auth`，公共契约仍留在 `application` |
| 新增业务事件类型 | `internal/event/event.go` |
| 更换或增加消息中间件 | `internal/event/<adapter>` |

不要为了“分层”给每个函数都创建接口。只有跨层边界、外部依赖或测试替身确实需要时才抽接口。

## 验证

```bash
rtk go test ./...
rtk go vet ./...
rtk bash ../scripts/validate-workspace.sh
```

Redis Lua 并发测试需要本地 Redis：

```bash
rtk env REDIS_ADDR=127.0.0.1:6380 go test ./internal/redcart/infrastructure/redis -run TestRateLimiterTokenBucketIsAtomic -count=1
```
