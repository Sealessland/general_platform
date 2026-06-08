package application

import (
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
)

type Service struct {
	repo       Repository
	aiProvider backendai.AIProvider
	now        func() time.Time
}

func NewService(repo Repository, aiProvider backendai.AIProvider) *Service {
	return &Service{
		repo:       repo,
		aiProvider: aiProvider,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}
