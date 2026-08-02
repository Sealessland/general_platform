# ADR 0007：JWT 与多实例运行边界

- 状态：已接受
- 日期：2026-08-02

## 背景

旧认证把随机 token 映射到 Redis session。它能共享登录态，但 token 本身没有标准声明，认证、缓存和仓储职责混在一起，本地 session 热缓存还会让跨实例登出存在短暂不一致。

项目需要一个能实际演示、同时适合初学者阅读的分布式最小闭环，而不是先拆出大量没有独立业务价值的微服务。

## 决策

### 1. 标准 JWT

- Access Token 与 Refresh Token 都使用 JWT、HS256 和 `golang-jwt/jwt/v5`。
- 标准 claims 包含 `iss`、`sub`、`aud`、`exp`、`iat`、`jti`；自定义 claims 只包含 `token_use`、角色、昵称、商家 ID 和会话 ID。
- Parser 固定只接受 HS256，同时校验 issuer、audience、过期时间与 token 类型，避免算法混淆和 Refresh Token 被当成 Access Token。
- `JWT_SECRET` 至少 32 字节。所有 API 实例必须使用相同 secret、issuer 和 audience；生产环境应由 Secret Manager 注入，不能使用 Compose 的本地默认值。

### 2. Redis 只保存共享状态

- Access JWT 的身份声明本地验签，不再把完整 token 或用户快照写入 Redis。
- Redis 保存一次性 Refresh 会话，以及已撤销 Access JWT 的 `jti`；key 中不出现原始 JWT。
- Refresh 轮换通过 Lua 原子完成“消费旧 Refresh、保存新 Refresh、撤销旧 Access”，并发刷新只有一个请求成功。
- 登出删除对应 Refresh 会话，并把 Access `jti` 加入黑名单直到 JWT 自然过期。
- Redis 不可用时认证 fail-closed，避免不同实例对撤销状态产生不同判断。

### 3. 两个 API 实例，一个入口

本地 Compose 运行 `backend:18081` 与 `backend-replica:18082`，Nginx 在 `18080` 使用 least-connections 转发。响应头 `X-RedCart-Upstream` 可用于观察当前请求落到哪个实例。

两个实例共享：

- PostgreSQL：业务真相、订单事务、幂等键和 Outbox。
- Redis：Refresh 会话、JWT 撤销状态、目录缓存和 Lua 限流。
- Kafka：Outbox 的异步发布目标。

数据库迁移按版本获取 PostgreSQL transaction advisory lock，避免两个实例同时执行同一份 DDL；Outbox relay 继续使用 `FOR UPDATE SKIP LOCKED` 协调并发领取。

## 代码边界

```text
interfaces/httpapi → application.TokenManager（中立契约）
                              ↑
              infrastructure/auth（JWT + Redis）

gateway → backend:18081 ─┐
                         ├→ PostgreSQL / Redis / Kafka
gateway → replica:18082 ─┘
```

JWT/Redis SDK 不能进入领域层。Repository 只负责业务数据，不再负责 token session。

## 非目标与限制

- 当前不做服务发现、自动扩缩容、Service Mesh、分布式事务或跨地域容灾。
- HS256 适合当前单一受信任后端；如果未来多个独立服务只需要验签，应迁移到非对称签名与 JWKS，并设计密钥轮换。
- JWT 中的角色变更最多延迟到 Access Token 过期后生效；高风险权限变更未来可增加用户级 token version。
- Redis 当前仍是单节点开发配置；代码使用 `UniversalClient` 保留接入 Sentinel/Cluster 的适配空间，但本次不宣称已完成 Redis 高可用。

## 验证

- JWT claims、HS256 白名单、token 类型隔离和弱 secret 拒绝测试。
- 两个独立 JWT Manager 共享 Redis 的跨实例认证与撤销测试。
- Refresh Token 单次消费、并发安全和旧 Access 立即撤销测试。
- PostgreSQL 双 Repository 并发启动测试。
- Compose 配置检查与网关实际请求验证。
