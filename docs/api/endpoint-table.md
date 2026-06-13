# API 接口表

本表是 `openapi.yaml` 的人工索引，便于按业务模块快速查接口。接口契约字段、schema 和示例以 `docs/api/openapi.yaml` 为准；如果接口行为变化，必须同步更新 OpenAPI 和本表。

## 约定

- 基础地址：`http://127.0.0.1:18080`
- 认证方式：`Authorization: Bearer <token>`
- `公开` 表示不需要登录。
- `可选登录` 表示未登录可访问，登录后会记录行为或返回登录相关上下文。
- 写操作必须使用表中声明的方法；状态变更接口不允许用 `GET` 触发。

## 基础与认证

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `GET` | `/healthz` | 公开 | 无 | `200` | 健康检查 |
| `POST` | `/api/auth/register` | 公开 | `RegisterRequest` | `201` | 注册消费者或商家演示账号，并返回登录态 |
| `POST` | `/api/auth/login` | 公开 | `LoginRequest` | `200` | 登录并获取 Bearer Token |
| `GET` | `/api/auth/me` | 登录用户 | Bearer Token | `200` | 获取当前登录用户信息 |

## 内容与商品

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `GET` | `/api/notes` | 公开 | 无 | `200` | 获取带挂载商品的笔记流 |
| `GET` | `/api/notes/{id}` | 可选登录 | `id` path 参数 | `200` | 获取笔记详情；登录后记录浏览事件 |
| `GET` | `/api/products` | 公开 | 无 | `200` | 获取上架商品列表 |
| `GET` | `/api/products/{id}` | 可选登录 | `id` path 参数 | `200` | 获取商品详情；登录后可记录商品点击事件 |
| `GET` | `/api/products/{id}/skus` | 公开 | `id` path 参数 | `200` | 获取商品 SKU 列表 |

## 购物车

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `GET` | `/api/cart` | 登录用户 | Bearer Token | `200` | 获取当前购物车 |
| `POST` | `/api/cart/items` | 登录用户 | `CartItemRequest` | `201` | 添加购物车商品 |
| `PUT` | `/api/cart/items/{id}` | 登录用户 | `id` path 参数，`CartItemUpdateRequest` | `200` | 修改购物车商品数量或勾选状态 |
| `DELETE` | `/api/cart/items/{id}` | 登录用户 | `id` path 参数 | `200` | 删除购物车商品 |

## 消费者订单

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `POST` | `/api/orders/preview` | 登录用户 | 可选 `CheckoutRequest` | `200` | 结算预览，校验金额与库存状态 |
| `GET` | `/api/orders` | 登录用户 | Bearer Token | `200` | 获取当前消费者订单列表 |
| `POST` | `/api/orders` | 登录用户 | `Idempotency-Key` header，`CheckoutRequest` | `201` | 使用幂等键创建订单；库存不足返回 `409` |
| `GET` | `/api/orders/{id}` | 登录用户 | `id` path 参数 | `200` | 获取订单详情 |
| `POST` | `/api/orders/{id}/pay` | 登录用户 | `id` path 参数 | `200` | 模拟支付 |
| `POST` | `/api/orders/{id}/cancel` | 登录用户 | `id` path 参数 | `200` | 取消未支付订单 |
| `POST` | `/api/orders/{id}/finish` | 登录用户 | `id` path 参数 | `200` | 确认收货并完成订单 |
| `POST` | `/api/orders/{id}/refund` | 登录用户 | `id` path 参数，可选 `RefundRequest` | `202` | 发起退款申请 |

## 商家商品与 SKU

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `GET` | `/api/merchant/products` | 商家 | Bearer Token | `200` | 获取商家商品列表 |
| `POST` | `/api/merchant/products` | 商家 | `MerchantProductRequest` | `201` | 创建商家商品 |
| `PUT` | `/api/merchant/products/{id}` | 商家 | `id` path 参数，`MerchantProductRequest` | `200` | 更新商家商品 |
| `POST` | `/api/merchant/products/{id}/skus` | 商家 | `id` path 参数，`MerchantSKURequest` | `201` | 为商品创建 SKU |
| `PUT` | `/api/merchant/skus/{id}` | 商家 | `id` path 参数，`MerchantSKURequest` | `200` | 更新 SKU |
| `POST` | `/api/merchant/products/{id}/online` | 商家 | `id` path 参数 | `200` | 商品上架 |
| `POST` | `/api/merchant/products/{id}/offline` | 商家 | `id` path 参数 | `200` | 商品下架 |

## 商家订单与看板

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `GET` | `/api/merchant/orders` | 商家 | Bearer Token | `200` | 获取商家订单列表 |
| `GET` | `/api/merchant/orders/{id}` | 商家 | `id` path 参数 | `200` | 获取商家订单详情 |
| `POST` | `/api/merchant/orders/{id}/ship` | 商家 | `id` path 参数，可选 `MerchantShipRequest` | `200` | 发货 |
| `POST` | `/api/merchant/orders/{id}/refund/approve` | 商家 | `id` path 参数 | `200` | 审批退款 |
| `GET` | `/api/merchant/dashboard/funnel` | 商家 | Bearer Token | `200` | 获取商家漏斗指标 |
| `GET` | `/api/merchant/dashboard/products` | 商家 | Bearer Token | `200` | 获取商家商品经营数据 |
| `GET` | `/api/merchant/dashboard/summary` | 商家 | Bearer Token | `200` | 获取商家经营汇总 |

## AI Copilot

| 方法 | 路径 | 权限 | 请求要点 | 成功响应 | 说明 |
|---|---|---|---|---|---|
| `POST` | `/api/ai/product-selling-points` | 商家 | `SellingPointRequest` | `200` | 生成商品卖点，并记录 AI 任务 |
| `POST` | `/api/ai/business-review` | 商家 | `BusinessReviewRequest` | `200` | 生成经营复盘草案，并记录 AI 任务 |
| `GET` | `/api/ai/tasks/{id}` | 任务所有者 | `id` path 参数 | `200` | 获取 AI 任务详情；按商家或用户所有权隔离 |
| `POST` | `/api/ai/a2ui` | 登录用户 | `A2UISurfaceRequest` | `200` | 生成 A2UI v0.9 声明式界面，前端按协议渲染 |

## 常见错误

| 状态码 | 语义 | 常见场景 |
|---|---|---|
| `400` | 请求参数错误 | JSON 无法解析、缺少必填字段、路径参数非法 |
| `401` | 未登录 | 缺少或无效 Bearer Token |
| `403` | 权限不足 | 非商家访问商家接口或 AI 商家能力 |
| `404` | 资源不存在 | 订单、商品、SKU、AI 任务不存在或不可见 |
| `405` | 方法不允许 | 使用错误 HTTP method 访问接口，尤其是用 `GET` 访问写操作 |
| `409` | 业务冲突 | 库存不足、非法订单状态流转、幂等或唯一约束冲突 |
