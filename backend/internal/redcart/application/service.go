package application

import (
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/event"
)

type Service struct {
	repo       Repository
	outbox     event.Outbox
	aiProvider backendai.AIProvider
	tokens     TokenManager
	now        func() time.Time
}

func NewService(repo Repository, aiProvider backendai.AIProvider, tokens TokenManager) *Service {
	outbox, _ := repo.(event.Outbox)
	return &Service{
		repo:       repo,
		outbox:     outbox,
		aiProvider: aiProvider,
		tokens:     tokens,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}
