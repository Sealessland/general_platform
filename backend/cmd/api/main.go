package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	"github.com/example/redcart-copilot/backend/internal/redcart/interfaces/httpapi"
)

func main() {
	repo, cleanup, err := initRepository()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
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

func initRepository() (application.Repository, func(), error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("POSTGRES_DSN is required")
	}

	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("initialize postgres repository: %w", err)
	}

	log.Printf("postgres repository connected")
	return repo, func() {
		if err := repo.Close(); err != nil {
			log.Printf("close postgres repository: %v", err)
		}
	}, nil
}
