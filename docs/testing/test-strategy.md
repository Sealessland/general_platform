# 测试策略

## 分层

- 领域层测试：订单状态机、金额规则、库存规则
- 应用层测试：注册登录、购物车选择结算、下单、幂等重试、支付、取消、退款、库存恢复、商家商品/SKU、dashboard、AI 任务状态
- 集成测试：PostgreSQL 迁移、Redis session/catalog 读侧适配器、事件链路
- 契约测试：OpenAPI 与 AI Provider 契约，AI Provider 覆盖商品卖点和经营复盘 mock 输出与非法输入
- HTTP 测试：认证、目录、购物车、结算、商家商品/SKU、商家订单、dashboard、AI 任务接口、非法输入和非法状态流转
- PostgreSQL-backed HTTP 测试：在 `RUN_POSTGRES_INTEGRATION=1` 时验证 Gin -> 应用层 -> PostgreSQL/GORM 的真实运行路径，包括主链路、并发库存、库存补偿、错误 method、越权和库存不足无副作用
- Redis 读侧测试：验证 session token 写入 Redis、TTL 解析、商品/SKU 缓存命中、跨实例 Redis 读取和写后失效
- 前端测试：类型检查、lint、构建与源码守卫，覆盖金额格式、库存计算、幂等键、购物车、dashboard 和 AI API 调用入口

## 质量门禁

- 后端：`go test ./...`
- 后端质量指标：`ci/scripts/backend-test-metrics.sh` 校验总覆盖率、关键包覆盖率、测试数量、benchmark 数量和 PostgreSQL benchmark 数量，并输出 CI artifact
- PostgreSQL 集成：`RUN_POSTGRES_INTEGRATION=1` 且提供 `POSTGRES_DSN` 后运行 PostgreSQL 仓储测试和 PostgreSQL-backed HTTP 测试
- 前端：`npm run typecheck`、`npm run lint`、`npm run build`
- AI 服务：`python -m compileall app tests`、`python -m unittest discover -s tests -v`、`python app/check_prompts.py`
- 仓库：`bash scripts/validate-workspace.sh`
- API：`bash scripts/check-openapi.sh`
- 项目 Codex 交付 hook：`python3 .codex/hooks/redcart_project_hook.py --mode quick`；高风险或跨层改动使用 `--mode full`

## 质量指标口径

后端 CI 的可靠测试指标以 `ci/artifacts/backend-test-metrics.json` 和 `ci/artifacts/backend-coverage-summary.txt` 为准。当前门禁阈值：

| 指标 | 阈值 | 当前口径 |
|---|---:|---|
| 总覆盖率 | 65.0% | `go tool cover -func backend/coverage.out` 的 total |
| 应用层覆盖率 | 80.0% | `backend/internal/redcart/application` |
| HTTP 层覆盖率 | 60.0% | `backend/internal/redcart/interfaces/httpapi` |
| 内存仓储覆盖率 | 90.0% | `backend/internal/redcart/infrastructure/memory` |
| PostgreSQL 仓储覆盖率 | 15.0% / 75.0% | `backend/internal/redcart/infrastructure/postgres`；普通 CI 覆盖离线 helper，`RUN_POSTGRES_INTEGRATION=1` 时要求真实数据库路径覆盖率至少 75.0% |
| AI 包覆盖率 | 95.0% | `backend/internal/ai` |
| 领域模型覆盖率 | 95.0% | `backend/internal/redcart/domain` |
| 后端测试数量 | 55 | `go test ./... -list '^Test'` 中的 `Test*` 数量 |
| 内存诊断 benchmark 数量 | 2 | `BenchmarkHTTPNotes`、`BenchmarkHTTPOrderPreview` |
| PostgreSQL benchmark 数量 | 2 | `BenchmarkHTTPPostgresOrderPreview`、`BenchmarkHTTPPostgresCreateOrder`，仅 `RUN_POSTGRES_INTEGRATION=1` 时要求 |

阈值按当前 MVP 稳定通过水平设置，目标是阻断覆盖率、测试规模和 benchmark 产物回退；后续功能稳定后应逐步提高阈值。

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

## 当前自动化覆盖

- `backend/internal/order/domain/order_status_test.go` 覆盖订单合法/非法状态流转、终态和库存释放状态。
- `backend/cmd/api/main_test.go` 覆盖 API 启动配置的环境变量 fallback 和缺失 `POSTGRES_DSN` 错误路径。
- `backend/internal/redcart/domain/models_test.go` 覆盖领域 clone helper 的拷贝隔离与空值语义。
- `backend/internal/redcart/infrastructure/memory/repository_test.go` 覆盖内存仓储的种子数据、clone 边界、用户/session/商家、目录读取、购物车、订单库存锁、事件列表和 AI task 基础路径。
- `backend/internal/redcart/infrastructure/postgres/repository_helpers_test.go` 覆盖 PostgreSQL 仓储适配器的 row scanner、nullable helper、迁移文件解析和 GORM result 适配，不依赖真实数据库。
- `backend/internal/redcart/infrastructure/postgres/repository_integration_test.go` 在 `RUN_POSTGRES_INTEGRATION=1` 时覆盖真实 PostgreSQL 下的用户/商家、笔记更新、商品/SKU、购物车、订单保存与列表、库存锁、行为事件和 AI task 读写。
- `backend/internal/redcart/infrastructure/redis/session_repository_test.go` 与 `catalog_cache_repository_test.go` 覆盖 Redis session token 存取、TTL 解析、商品/SKU 热缓存、跨实例 Redis 读取和写后失效行为。
- `backend/internal/redcart/application/service_test.go` 以及 `service_auth_additional_test.go`、`service_checkout_additional_test.go`、`service_order_additional_test.go`、`service_merchant_dashboard_additional_test.go`、`service_ai_additional_test.go` 覆盖幂等下单、退款库存恢复、注册/登录、内容/商品读取、购物车选择结算、checkout 校验、订单权限边界、非法状态流转、商家商品/SKU、dashboard、AI 任务成功/失败落库。
- `backend/internal/redcart/interfaces/httpapi/server_test.go`、`server_cart_test.go`、`server_orders_test.go`、`server_merchant_test.go`、`server_ai_test.go`、`server_postgres_test.go`、`server_benchmark_test.go` 和 `server_test_helpers_test.go` 覆盖消费者主链路、退款与 AI 生成、写操作 method gate、未登录拒绝、基础路由/CORS/404、认证和目录负向、购物车增删改、订单列表/详情、订单非法输入和状态冲突、商家商品/SKU、dashboard、AI business review、任务读取边界、PostgreSQL-backed HTTP 集成路径和 HTTP benchmark。
- PostgreSQL-backed HTTP 测试在 `RUN_POSTGRES_INTEGRATION=1` 时覆盖真实 Gin -> 应用层 -> PostgreSQL/GORM 路径，包括主链路、并发库存、库存补偿、库存不足无副作用、错误 method 和越权。
- `frontend/tests/app.test.mjs` 是源码守卫，确认前端仍使用整数分金额、可售库存扣减 locked stock、结算幂等键、购物车、dashboard 和 AI 入口。
- `ai-service/tests/test_provider.py` 覆盖 mock selling points、business review 以及非法输入。
- `ci/scripts/backend-test-metrics.sh` 将覆盖率、测试数量和 benchmark 数量固化为 CI 门禁，并上传 `backend-test-metrics.json`、`backend-coverage-summary.txt`、`backend-coverage-functions.txt` 和 `backend-test-list.txt`。

## 已知测试边界

- 前端尚未引入浏览器级 E2E 自动化；当前验证是源码守卫、构建和后端 HTTP 集成测试。
- QPS 基线使用 Go `httptest` benchmark 直接打 handler，不是经过真实网络端口的外部压测。
- `scripts/validate-workspace.sh` 是结构和文档入口检查，不替代业务测试。
