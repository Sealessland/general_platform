# 性能基线记录

本记录只保存当前运行时性能基线。当前运行时是 Gin + GORM + PostgreSQL，因此内存仓储 benchmark 不进入本基线。

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
| `BenchmarkHTTPPostgresOrderPreview-8` | 6962.58 | 143625 | Gin -> 应用层 -> PostgreSQL/GORM 读多查询路径 |
| `BenchmarkHTTPPostgresCreateOrder-8` | 70.96 | 14091653 | Gin -> 应用层 -> PostgreSQL/GORM 写路径，覆盖幂等下单、事务、库存条件更新、订单明细和库存锁写入 |

## 评估结论

- 本基线数据来自 Gin handler -> 应用层 -> GORM PostgreSQL driver -> 本地 Docker Compose PostgreSQL 的真实运行路径。
- `backend-qps.txt` 是内存仓储下的诊断数据，不进入运行时 baseline，也不用于评估数据库迁移成果。
- 结算预览在 PostgreSQL-backed 路径约 6960 QPS，读路径表现明显好于上一次基线，仍可作为当前 MVP 的可演示基线。
- 下单写路径约 71 QPS，主要覆盖多次查询、事务、条件更新、订单明细写入、库存锁写入和后续事件写入，仍然是后续优化的重点。
- `CreateOrder` 路径约比 `OrderPreview` 慢两个数量级，并伴随 `69457 B/op`、`1081 allocs/op`，说明写路径的对象分配和数据库往返成本都偏高。
- 本轮 `redcart.backend` 的 CPU profile 顶部主要落在 `runtime.schedule`、`runtime.findRunnable`、`runtime.notesleep`、`runtime.futexsleep`、`runtime.gcBgMarkWorker`、`runtime.gcDrain` 和 `net/http` 写出路径；这说明在当前压力模型下，调度与 GC 开销已经足够明显，单次采样里没有业务函数压过 runtime 热点。
- Pyroscope 容器启动后存在 `metastore`、`ingester`、`segment writer` 的 staged readiness 窗口，本地人工复核前必须确认服务已经 ready，不能只看容器 `Up`。
- 新增的 PostgreSQL-backed HTTP 集成测试已经覆盖注册、登录、商品/SKU、上架、结算预览、幂等下单、库存预锁、支付确认、发货、完成、看板和 AI 任务读取。
- 新增的 PostgreSQL-backed HTTP 并发测试确认库存为 1 的 SKU 在 24 个并发下单请求下只能创建 1 笔订单，其余请求返回 `409 conflict`。
- 新增的 PostgreSQL-backed HTTP 反向路径测试已经覆盖取消释放库存、支付后退款恢复库存、库存不足无副作用、错误 method 不触发状态变化、消费者访问商家接口、跨用户读取订单和跨商家读取 AI 任务。

## 2026-06-08 Pyroscope 可用性复核口径

本地复核结论：Pyroscope 在当前 Docker Compose 运行路径下可用。后端日志显示 `pyroscope profiling enabled for redcart.backend -> http://127.0.0.1:4040`，并且可以通过本地 Pyroscope UI 人工检查 `redcart.backend` 的 profile 数据。

Pyroscope profile 查询不进入 GitHub Actions 门禁，也不使用 curl 查询 profile API 作为自动化证据。CI 只验证后端 profiling 配置解析、默认关闭路径、启动失败传播和常规业务/性能门禁；profile 诊断保留为本地手动复核能力。

仍保留的边界：mutex、block 等 profile types 尚未作为性能诊断基线使用。

## 后续优化方向

- 为 `CreateOrder` 路径拆分数据库查询次数和写入阶段耗时，定位慢点是预览构建、事务写入、事件写入还是订单视图回填。
- 补 query plan 检查，优先关注 SKU、订单、库存锁和行为事件查询。
- 如果写路径需要更高吞吐，再评估 Redis Lua 预扣与 PostgreSQL 最终落盘的 ADR 方案。
