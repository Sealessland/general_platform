# 初学者教程：跑通分布式 RedCart

这篇教程只做一件事：在本机启动两个后端实例，并亲眼验证 JWT 登录态能跨实例使用和撤销。

## 1. 先认识请求路径

```text
浏览器或 curl
    -> Nginx :18080
        -> backend-a :18081
        -> backend-b :18082
            -> PostgreSQL（业务真相）
            -> Redis（Refresh 会话、撤销、缓存、限流）
            -> Kafka（Outbox 事件）
```

初学者先从 `backend/README.md` 看目录，再回到这里运行系统；不需要先读完所有源码。

## 2. 准备环境

需要 Docker Compose、curl 和 bash。复制配置并生成至少 32 字节的随机 JWT 密钥：

```bash
cp .env.example .env
openssl rand -base64 48
```

把输出填入 `.env` 的 `JWT_SECRET`，不要提交 `.env`。然后启动完整环境：

```bash
bash scripts/local-dev.sh
docker compose ps
```

健康检查：

```bash
curl -i http://127.0.0.1:18080/healthz
```

连续请求时观察 `X-RedCart-Upstream`，它会显示请求落到哪个后端实例。

## 3. 验证跨实例认证

仓库提供了可重复脚本：

```bash
bash scripts/verify-distributed-auth.sh
```

脚本会在一个实例登录、另一个实例读取身份和登出，再回到第一个实例确认 Access Token 已撤销。成功说明：

- Access Token 是标准 JWT，可由任一实例本地验签；
- Refresh 会话和撤销状态放在共享 Redis，而不是某个进程内存；
- 多实例迁移和种子数据由 PostgreSQL advisory lock 串行保护。

JWT payload 可以在本地解码观察，但“能解码”不等于“验签通过”，不要把 token 粘贴到第三方网站。

## 4. 观察 Kafka 与限流

订单状态事件先和业务事务一起写入 PostgreSQL Outbox，再由 relay 发布到 Kafka。可查看容器日志：

```bash
docker compose logs --tail=100 backend-a backend-b kafka
```

Redis Lua 令牌桶位于 `backend/internal/ratelimit`，HTTP 策略装配位于 `interfaces/httpapi/middleware_rate_limit.go`。触发限流时响应为 `429`，并带 `Retry-After` 和剩余额度响应头。

## 5. 停止环境

```bash
docker compose down
```

下一步建议阅读 `docs/adr/0007-jwt-and-multi-instance-runtime.md`，再从登录 handler 跟到 `application.TokenManager` 和 `infrastructure/auth`。
