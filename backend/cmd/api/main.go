// Package main 是 redcart API 服务入口：负责按依赖顺序装配基础设施
// （profiler、仓储、AI provider、outbox 转发器）并启动 HTTP 服务。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	aigrpc "github.com/example/redcart-copilot/backend/internal/ai/grpc"
	"github.com/example/redcart-copilot/backend/internal/event"
	kafkaevent "github.com/example/redcart-copilot/backend/internal/event/kafka"
	"github.com/example/redcart-copilot/backend/internal/event/outbox"
	"github.com/example/redcart-copilot/backend/internal/observability"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
	"github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi"
)

// main 装配各组件并启动 HTTP 服务；任一关键依赖初始化失败都会直接退出进程。
func main() {
	stopProfiler, err := startProfilerFromEnv(pyroscopeStart, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer stopProfiler()

	stopTracing, err := observability.SetupTracing(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := stopTracing(shutdownCtx); err != nil {
			log.Printf("shut down tracing: %v", err)
		}
	}()

	repo, limiter, cleanup, err := initRepository(log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	aiProvider, err := newAIProvider()
	if err != nil {
		log.Fatal(err)
	}
	service := application.NewService(repo, aiProvider)
	rateLimit, err := newRateLimitMiddleware(limiter, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", envOrDefault("HTTP_PORT", "18080")),
		Handler:           observability.HTTPHandler(httpapi.NewServerWithRateLimit(service, rateLimit).Handler()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if outboxRelay, ok := repo.(event.OutboxRelayStore); ok {
		stopOutbox := startOutboxPublisher(outboxRelay, log.Default())
		defer stopOutbox()
	}

	log.Printf("redcart api listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// startOutboxPublisher 根据 KAFKA_BROKERS 决定是否启用 outbox 后台转发：
// 未配置 brokers 时返回空操作，保证无 Kafka 的环境也能正常启动。
// 返回的闭包用于停止转发器并关闭发布连接。
func startOutboxPublisher(store event.OutboxRelayStore, logger *log.Logger) func() {
	brokers := kafkaevent.ParseBrokers(envOrDefault("KAFKA_BROKERS", ""))
	if len(brokers) == 0 {
		return func() {}
	}
	publisher, err := kafkaevent.NewPublisher(brokers, kafkaevent.PublisherConfig{
		TopicPrefix: envOrDefault("KAFKA_TOPIC_PREFIX", "redcart.events"),
		Logger:      logger,
	})
	if err != nil {
		logger.Printf("kafka publisher disabled: %v", err)
		return func() {}
	}
	relay := outbox.NewPublisher(store, publisher, outbox.Config{
		Interval:  5 * time.Second,
		BatchSize: 100,
		Logger:    logger,
	})
	relay.Start()
	return func() {
		relay.Stop()
		_ = publisher.Close()
	}
}

// newAIProvider 根据 AI_PROVIDER 环境变量选择 AI 实现：
// "grpc" 走 gRPC 客户端（默认地址 127.0.0.1:50051），其余值使用 MockProvider。
func newAIProvider() (backendai.AIProvider, error) {
	switch os.Getenv("AI_PROVIDER") {
	case "grpc":
		addr := envOrDefault("AI_GRPC_ADDR", "127.0.0.1:50051")
		return aigrpc.NewClient(addr)
	default:
		return backendai.MockProvider{}, nil
	}
}

// newRateLimitMiddleware 从环境变量解析写接口与 AI 接口的限流策略并构造
// 中间件；策略非法时返回错误（启动即失败，避免带病运行）。
func newRateLimitMiddleware(limiter *redisrepo.Limiter, logger *log.Logger) (*httpapi.RateLimitMiddleware, error) {
	writePolicy, err := redisrepo.RateLimitPolicyFromEnv(
		"RATE_LIMIT_WRITE",
		os.Getenv("RATE_LIMIT_WRITE_RATE"), os.Getenv("RATE_LIMIT_WRITE_BURST"),
		redisrepo.Policy{RatePerSecond: redisrepo.DefaultWriteRate, Burst: redisrepo.DefaultWriteBurst},
	)
	if err != nil {
		return nil, err
	}
	aiPolicy, err := redisrepo.RateLimitPolicyFromEnv(
		"RATE_LIMIT_AI",
		os.Getenv("RATE_LIMIT_AI_RATE"), os.Getenv("RATE_LIMIT_AI_BURST"),
		redisrepo.Policy{RatePerSecond: redisrepo.DefaultAIRate, Burst: redisrepo.DefaultAIBurst},
	)
	if err != nil {
		return nil, err
	}
	return httpapi.NewRateLimitMiddleware(limiter, logger, writePolicy, aiPolicy), nil
}

// envOrDefault 读取环境变量，未设置或值为空时返回 fallback。
func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
