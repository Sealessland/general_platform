# 2026-06-05 测试问题归档

## 背景

本记录归档一次 Docker/Podman 环境下的项目健全性彻查结果。范围包括后端 HTTP 主链路、权限边界、库存一致性、AI 任务隔离、仓库级验证和 Compose 运行状态。

本次不修改业务代码，仅记录可复现问题、证据和后续修复建议。

## 已通过基线

- `rtk bash scripts/validate-workspace.sh` 通过。
- `rtk bash scripts/check-openapi.sh` 通过。
- 前端 Node 容器内 `npm ci`、`npm run typecheck`、`npm run lint`、`npm test`、`npm run build` 通过。
- AI service Python 容器内编译、单元测试和 prompt 检查通过。
- 后端 Go 容器内 `gofmt`、`go vet`、`go test ./...`、PostgreSQL 集成测试、构建和 benchmark 通过。
- Compose 中 `backend`、`frontend`、`postgres` 可启动；`/healthz` 返回 `{"status":"ok"}`；前端首页 HTTP 200。
- 真实 HTTP 主链路冒烟通过：登录、创建商品/SKU、上架、加购、预览、幂等下单、支付、发货、完成、退款审批、商家看板、AI 任务查询。

## 已确认问题

### 1. GET 请求可触发写操作

严重级别：高。

现象：部分状态变更路由只按 URL 后缀分支，没有在分支内校验 HTTP method，因此 `GET` 请求也可以执行支付、取消、完成、退款、商品上下架、商家发货、退款审批等写操作。

源码位置：

- `backend/internal/redcart/interfaces/httpapi/server.go:361`
- `backend/internal/redcart/interfaces/httpapi/server.go:364`
- `backend/internal/redcart/interfaces/httpapi/server.go:376`
- `backend/internal/redcart/interfaces/httpapi/server.go:388`
- `backend/internal/redcart/interfaces/httpapi/server.go:400`
- `backend/internal/redcart/interfaces/httpapi/server.go:486`
- `backend/internal/redcart/interfaces/httpapi/server.go:498`
- `backend/internal/redcart/interfaces/httpapi/server.go:570`
- `backend/internal/redcart/interfaces/httpapi/server.go:573`
- `backend/internal/redcart/interfaces/httpapi/server.go:590`

复现证据：

```text
{"get_online_status":"online","get_pay_status":"PAID","consumer_ai_task_id":3,"other_consumer_read_task_status":"completed"}
```

其中 `get_online_status=online` 表示 `GET /api/merchant/products/{id}/online` 完成了上架；`get_pay_status=PAID` 表示 `GET /api/orders/{id}/pay` 完成了支付。

影响：违反 HTTP 语义和 OpenAPI 契约，容易被浏览器预取、爬虫、缓存层或误触发请求造成订单和商品状态被改变。

建议：所有写操作分支显式要求 `POST`，不符合 method 时返回 405；补 HTTP 层回归测试覆盖每个状态变更路由的 GET/POST 行为。

### 2. AI Copilot 权限边界和任务隔离不足

严重级别：高。

现象：PRD 将 AI Copilot 定义为商家能力，但后端生成接口只要求登录，没有校验 `actor.Role == merchant`。消费者也能创建 AI 任务。任务读取逻辑允许 `task.MerchantID == actor.MerchantID`，而普通消费者的 `MerchantID` 默认为 0，导致一个消费者创建的 AI 任务可被另一个消费者读取。

源码位置：

- `backend/internal/redcart/application/service.go:949`
- `backend/internal/redcart/application/service.go:993`
- `backend/internal/redcart/application/service.go:1043`
- `docs/prd/003-ai-copilot.md:7`

复现证据：

```text
{"consumer_ai_task_id":3,"other_consumer_read_task_status":"completed"}
```

含义：消费者 A 创建的 AI task ID 3，被消费者 B 读取成功。

影响：AI 任务输入/输出可能包含商品、经营、评价或人工修正信息，存在横向越权读取风险。

建议：AI 生成接口强制商家角色；任务读取使用更严格的所有权判断，例如商家任务按 `merchant_id` 隔离，非商家任务只允许 `user_id` 精确匹配，避免 `merchant_id=0` 作为共享匹配条件。

### 3. 并发下单可绕过库存约束

严重级别：高。

现象：创建订单时先读取 `stock - locked_stock`，再保存订单、更新 SKU、写库存锁。PostgreSQL 路径没有把订单、库存预锁和库存锁写入放在同一个事务里，也没有行级锁或原子条件更新。并发请求下，同一个库存为 1 的 SKU 可以创建多笔订单。

源码位置：

- `backend/internal/redcart/application/service.go:301`
- `backend/internal/redcart/application/service.go:355`
- `backend/internal/redcart/application/service.go:359`
- `backend/internal/redcart/application/service.go:362`
- `backend/internal/redcart/application/service.go:366`
- `backend/internal/redcart/infrastructure/postgres/repository.go:425`
- `backend/internal/redcart/infrastructure/postgres/repository.go:557`
- `backend/internal/redcart/infrastructure/postgres/repository.go:651`
- `docs/adr/0003-inventory-lock-strategy.md:11`
- `docs/architecture/inventory-design.md:20`

复现证据：

```text
{
  "product_id":5,
  "sku_id":6,
  "created_count":14,
  "created_order_ids":[8,7,19,20,14,12,18,13,16,9,11,15,17,10],
  "conflict_count":18,
  "other_count":0,
  "sku_after":{"id":6,"stock":1,"locked_stock":3,"status":"active"}
}
```

测试条件：新建库存为 1 的 SKU，32 个并发请求使用不同 `Idempotency-Key` 下单。实际创建 14 笔订单。

影响：会造成超卖、库存锁异常、订单履约不可控。最终 `locked_stock=3` 也说明并发写入存在丢失更新，不只是业务校验漏判。

建议：短期在 PostgreSQL 中使用事务、`SELECT ... FOR UPDATE` 或 `UPDATE product_skus SET locked_stock = locked_stock + ? WHERE id = ? AND stock - locked_stock >= ?` 的原子条件更新；中期按 ADR 落地 Redis Lua 预扣和 PostgreSQL 最终事务落盘；补并发集成测试。

## 其他风险

- 当前 `validate-workspace.sh` 偏结构检查，不会发现上述业务行为问题。
- OpenAPI 快速检查只 grep 关键路径，不校验 method、schema 与实现一致性。
- 后端测试覆盖了正常主链路，但没有覆盖错误 method、横向越权、消费者调用 AI、并发库存。
- PostgreSQL 集成测试只验证连接和少量种子数据，没有验证事务一致性和并发。
- 前端测试是源码字符串级检查，不能覆盖真实 DOM 交互和 API 错误状态。

## 建议修复顺序

1. 先修 HTTP method gate，并补 HTTP 层回归测试。
2. 修 AI Copilot 角色校验和任务所有权判断，并补越权测试。
3. 修 PostgreSQL 库存预锁事务和并发控制，并补并发集成测试。
4. 扩展 OpenAPI/CI 检查，让 method、权限和关键错误路径进入可执行门禁。

## 2026-06-06 修复推进记录

本节记录本归档中已确认问题的修复状态，避免风险只存在于聊天上下文或临时脚本中。

### 1. GET 请求触发写操作

状态：已修复。

修复内容：

- 在订单支付、取消、完成、退款申请路由中显式要求 `POST`。
- 在商品上下架、商家发货、退款审批路由中显式要求 `POST`。
- 非 `POST` 写操作请求返回 `405 method_not_allowed`，且不改变订单或商品状态。

回归测试：

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run 'TestStateChangingRoutesRejectGET'
```

### 2. AI Copilot 权限边界和任务隔离不足

状态：已修复。

修复内容：

- `GenerateSellingPoints` 与 `GenerateBusinessReview` 一致，要求商家角色。
- AI 任务读取改为按角色判断所有权：商家只能按非 0 `merchant_id` 读取本店任务；消费者只能按 `user_id` 精确读取自己的任务。
- 避免 `merchant_id=0` 使多个消费者共享匹配条件。

回归测试：

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/application -run 'TestConsumerCannot'
```

### 3. 并发下单绕过库存约束

状态：已修复 PostgreSQL 运行路径。

修复内容：

- 新增仓储契约 `SaveOrderWithInventoryLocks`，把订单创建、订单明细、库存预锁和库存锁记录作为一个原子操作。
- PostgreSQL 实现使用事务和条件更新：
  `UPDATE product_skus SET locked_stock = locked_stock + ? WHERE id = ? AND stock - locked_stock >= ?`
- 内存实现使用仓储互斥锁在同一个临界区内完成库存校验、订单保存和库存锁写入。
- 服务层把仓储返回的库存不足映射为 `409 conflict`。

回归测试：

```bash
rtk env GOCACHE=/tmp/go-build-cache POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 go test ./internal/redcart/infrastructure/postgres -run 'TestConcurrentCreateOrderReservesStockAtomically' -count=1 -v
```

补充验证：

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./...
```

## 剩余风险

- OpenAPI 快速检查仍是关键词级检查，尚不能证明 method、权限和错误响应与实现完全一致。
- PostgreSQL 并发测试已覆盖单 SKU 库存为 1 的超卖场景；多 SKU 订单、支付确认、取消释放和退款恢复仍建议继续扩展集成测试。
- Redis Lua 预扣仍是下一阶段目标；当前修复先保证 PostgreSQL MVP 运行路径不超卖。

## 本次临时验证脚本

临时脚本仅用于本轮排查，位于 `/tmp`，未纳入仓库：

- `/tmp/redcart_docker_smoke.py`
- `/tmp/redcart_verify_findings.py`
- `/tmp/redcart_concurrency_check.py`

如需长期保留，应整理为仓库内正式测试，并避免依赖当前演示数据库残留数据。
