package main

import (
	"log"
	"net/http"
	"os"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	"github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi"
)

func main() {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
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

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
