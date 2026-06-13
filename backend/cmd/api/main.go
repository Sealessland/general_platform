package main

import (
	"log"
	"net/http"
	"os"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	aigrpc "github.com/example/redcart-copilot/backend/internal/ai/grpc"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi"
)

func main() {
	stopProfiler, err := startProfilerFromEnv(pyroscopeStart, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer stopProfiler()

	repo, cleanup, err := initRepository(log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	aiProvider, err := newAIProvider()
	if err != nil {
		log.Fatal(err)
	}
	service := application.NewService(repo, aiProvider)
	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", envOrDefault("HTTP_PORT", "18080")),
		Handler:           httpapi.NewServer(service).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("redcart api listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
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
