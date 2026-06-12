# 性能基线记录

本记录只保存当前运行时性能基线。当前运行时是 Gin + PostgreSQL 适配层；GORM 仅保留建连与迁移职责，因此内存仓储 benchmark 不进入本基线。

## 2026-06-06 Gin/GORM 迁移评估

测试环境：

- CPU：11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz
- PostgreSQL：Docker Compose 中的 `postgres:16`，本地端口 `127.0.0.1:15432`
- 命令：`rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=2s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh`

## 2026-06-08 Pyroscope 接入后复测

测试环境：

- CPU：11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz
- 启动方式：`rtk docker compose up -d --build --remove-orphans postgres pyroscope backend frontend`
- PostgreSQL：Docker Compose 中的 `postgres:16`，本地端口 `127.0.0.1:15432`
- Profiling：后端通过 Go push mode 上报到本地 `http://127.0.0.1:4040`
- Benchmark 命令：`rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=2s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh`

## 当前运行时数据

| Benchmark | QPS | ns/op | 口径 |
|---|---:|---:|---|
| `BenchmarkHTTPPostgresOrderPreview-8` | 6962.58 | 143625 | Gin -> 应用层 -> PostgreSQL 读多查询路径 |
| `BenchmarkHTTPPostgresCreateOrder-8` | 70.96 | 14091653 | Gin -> 应用层 -> PostgreSQL 写路径，覆盖幂等下单、事务、库存条件更新、订单明细和库存锁写入 |

## 2026-06-10 CreateOrder 关键路径响应组装优化

优化边界：

- 仅修改 `backend/internal/redcart/application/service_order.go`
- `backend/internal/redcart/application/service_order_helpers.go`
- `backend/internal/redcart/infrastructure/{memory,postgres}/repository.go`
- 不调整 OpenAPI、schema、库存规则、支付/退款状态机或 Redis 读侧

优化原因：

- `CreateOrder` 首次成功返回时，会额外调用 `ListOrderEvents(order.ID)` 和 `ListInventoryLocksByOrder(order.ID)`，只是为了把刚写入的事件和库存锁重新读一遍再返回给 HTTP。
- 当请求体显式提供 `items` 时，这条路径仍会无条件执行一次 `DeleteSelectedCartItems(actor.UserID)`，而 benchmark 场景并不依赖购物车结算。

优化动作：

- 将订单创建成功响应改为直接复用刚写出的订单项、`ORDER_CREATED` 事件和库存锁，不再为这两部分做额外仓储回查。
- 让内存仓储和 PostgreSQL 仓储在 `SaveOrderWithInventoryLocks` 内把库存锁 `ID/OrderID` 回填到调用方传入切片，保证应用层可直接构造返回值。
- 仅在 `CreateOrder` 实际从“已选购物车项”结算时，才执行 `DeleteSelectedCartItems`；显式传入 `items` 的直接购买路径不再做无意义删除。

复测环境：

- CPU：11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz
- PostgreSQL：本地 `postgres:16`，端口 `127.0.0.1:15432`
- 基准命令：`rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^$' -bench BenchmarkHTTPPostgresCreateOrder -benchmem -count=1 -benchtime=2s`

复测数据：

| Benchmark | 优化前 QPS | 优化后 QPS | 优化前 ns/op | 优化后 ns/op | 优化前 B/op | 优化后 B/op | 优化前 allocs/op | 优化后 allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `BenchmarkHTTPPostgresCreateOrder-8` | 59.46 | 73.16 | 16816738 | 13667673 | 34890 | 31426 | 587 | 495 |

复测结论：

- `CreateOrder` 关键路径 QPS 提升约 `23.0%`。
- 单次请求分配字节下降约 `9.9%`，分配次数下降约 `15.7%`。
- 收益主要来自减少首次成功创建订单响应里的冗余回查，以及避开显式下单路径上的无意义购物车删除。

## 2026-06-08 PostgreSQL 写路径适配层优化复测

优化边界：

- 仅修改 `backend/internal/redcart/infrastructure/postgres/repository.go`
- 不调整应用层、HTTP 层、内存仓储、OpenAPI、migration 或 schema

优化原因：

- `CreateOrder` 的 PostgreSQL 写路径在适配层里存在额外数据库往返：部分 `UPDATE/INSERT` 结束后又立刻按主键回读完整对象。
- 运行时查询仍经过 GORM `Raw/Exec` 包装，带来额外分配和调用开销，而这条路径本身已经是纯 SQL 仓储。

优化动作：

- 将 PostgreSQL 适配层运行时 SQL 调用切到 `database/sql`，保留 GORM 只负责建连与迁移。
- 用 `INSERT/UPDATE ... RETURNING` 直接回填 `id`、`created_at`、`updated_at`，去掉 `SaveProduct`、`SaveSKU`、`SaveCartItem`、`SaveOrder`、`SaveOrderWithInventoryLocks` 的写后回读。
- 订单写入事务内直接回填 `order_items.id` 和时间戳，不再在提交后整单重查。
- `SaveOrderWithInventoryLocks` 在只有一个 lock 时不再复制和排序切片。

复测环境：

- CPU：11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz
- PostgreSQL：Docker Compose 中的 `postgres:16`，本地端口 `127.0.0.1:15432`
- 基准命令：`rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=2s GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^$' -bench BenchmarkHTTPPostgresCreateOrder -benchmem`

复测数据：

| Benchmark | 优化前 QPS | 优化后 QPS | 优化前 ns/op | 优化后 ns/op | 优化前 B/op | 优化后 B/op | 优化前 allocs/op | 优化后 allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `BenchmarkHTTPPostgresCreateOrder-8` | 69.96 | 73.67 | 14294460 | 13573929 | 69576 | 35195 | 1096 | 609 |

复测结论：

- `CreateOrder` 写路径 QPS 提升约 `5.3%`。
- 单次请求分配字节下降约 `49%`，分配次数下降约 `44%`。
- 这次收益主要来自适配层内部的数据库往返减少和 GORM 运行时包装开销减少，不涉及业务语义变化。

## 评估结论

- 本基线数据来自 Gin handler -> 应用层 -> PostgreSQL 仓储适配层 -> 本地 Docker Compose PostgreSQL 的真实运行路径。
- `backend-qps.txt` 是内存仓储下的诊断数据，不进入运行时 baseline，也不用于评估数据库迁移成果。
- 结算预览在 PostgreSQL-backed 路径约 6960 QPS，读路径表现明显好于上一次基线，仍可作为当前 MVP 的可演示基线。
- 下单写路径约 71 QPS，主要覆盖多次查询、事务、条件更新、订单明细写入、库存锁写入和后续事件写入，仍然是后续优化的重点。
- `CreateOrder` 路径约比 `OrderPreview` 慢两个数量级，并伴随 `69457 B/op`、`1081 allocs/op`，说明写路径的对象分配和数据库往返成本都偏高。
- 本轮 `redcart.backend` 的 CPU profile 顶部主要落在 `runtime.schedule`、`runtime.findRunnable`、`runtime.notesleep`、`runtime.futexsleep`、`runtime.gcBgMarkWorker`、`runtime.gcDrain` 和 `net/http` 写出路径；这说明在当前压力模型下，调度与 GC 开销已经足够明显，单次采样里没有业务函数压过 runtime 热点。
- Pyroscope 容器启动后存在 `metastore`、`ingester`、`segment writer` 的 staged readiness 窗口，本地人工复核前必须确认服务已经 ready，不能只看容器 `Up`。
- 新增的 PostgreSQL-backed HTTP 集成测试已经覆盖注册、登录、商品/SKU、上架、结算预览、幂等下单、库存预锁、支付确认、发货、完成、看板和 AI 任务读取。
- 新增的 PostgreSQL-backed HTTP 并发测试确认库存为 1 的 SKU 在 24 个并发下单请求下只能创建 1 笔订单，其余请求返回 `409 conflict`。
- 新增的 PostgreSQL-backed HTTP 反向路径测试已经覆盖取消释放库存、支付后退款恢复库存、库存不足无副作用、错误 method 不触发状态变化、消费者访问商家接口、跨用户读取订单和跨商家读取 AI 任务。
- 2026-06-08 的 PostgreSQL 写路径优化只触碰适配层，不改变事务边界、库存条件更新语义或接口契约；全量后端门禁和 PostgreSQL 并发下单测试均已通过。

## 2026-06-08 Pyroscope 可用性复核口径

本地复核结论：Pyroscope 在当前 Docker Compose 运行路径下可用。后端日志显示 `pyroscope profiling enabled for redcart.backend -> http://127.0.0.1:4040`，并且可以通过本地 Pyroscope UI 人工检查 `redcart.backend` 的 profile 数据。

Pyroscope profile 查询不进入 GitHub Actions 门禁，也不使用 curl 查询 profile API 作为自动化证据。CI 只验证后端 profiling 配置解析、默认关闭路径、启动失败传播、可选 mutex/block 采样配置和常规业务/性能门禁；profile 诊断保留为本地手动复核能力。

仍保留的边界：mutex、block 等 profile types 已可通过环境变量临时启用，但尚未作为性能诊断基线固定采样。

## 后续优化方向

- 为 `CreateOrder` 路径拆分数据库查询次数和写入阶段耗时，定位慢点是预览构建、事务写入、事件写入还是订单视图回填。
- 补 query plan 检查，优先关注 SKU、订单、库存锁和行为事件查询。
- 如果写路径需要更高吞吐，再评估 Redis Lua 预扣与 PostgreSQL 最终落盘的 ADR 方案。
