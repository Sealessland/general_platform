package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	aigrpc "github.com/example/redcart-copilot/backend/internal/ai/grpc"
	"github.com/example/redcart-copilot/backend/internal/event"
	kafkaevent "github.com/example/redcart-copilot/backend/internal/event/kafka"
	"github.com/example/redcart-copilot/backend/internal/event/outbox"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi"
)

func main() {
	stopProfiler, err := startProfilerFromEnv(pyroscopeStart, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer stopProfiler()

	dependencies, err := initDependencies(log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer dependencies.close()
	aiProvider, err := newAIProvider()
	if err != nil {
		log.Fatal(err)
	}
	service := application.NewService(dependencies.repository, aiProvider, dependencies.tokenManager)
	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", envOrDefault("HTTP_PORT", "18080")),
		Handler:           httpapi.NewServer(service, httpapi.WithRateLimiter(dependencies.rateLimiter)).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	stopOutbox := startOutboxPublisher(dependencies.outbox, log.Default())
	defer stopOutbox()

	log.Printf("redcart api listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func startOutboxPublisher(store event.OutboxRelayStore, logger *log.Logger) func() {
	brokers := splitCSV(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return func() {}
	}
	publisher, err := kafkaevent.NewPublisher(brokers, envOrDefault("KAFKA_TOPIC", "redcart.events"))
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
	return func() { relay.Stop(); publisher.Close() }
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func newAIProvider() (backendai.AIProvider, error) {
	switch os.Getenv("AI_PROVIDER") {
	case "grpc":
		addr := envOrDefault("AI_GRPC_ADDR", "127.0.0.1:50051")
		return aigrpc.NewClient(addr)
	default:
		return backendai.MockProvider{}, nil
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
