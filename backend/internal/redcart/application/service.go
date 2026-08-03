package application

import (
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/event"
)

// Service 聚合应用用例，编排仓储、事件与 AI 依赖；具体业务方法按域拆分在 service_*.go 中。
type Service struct {
	repo       Repository
	outbox     event.Outbox
	aiProvider backendai.AIProvider
	now        func() time.Time
}

// NewService 构造应用服务；若 repo 同时实现 event.Outbox（如 postgres 仓储），
// 则为订单状态流转开启事务内事件写出能力。
func NewService(repo Repository, aiProvider backendai.AIProvider) *Service {
	outbox, _ := repo.(event.Outbox)
	return &Service{
		repo:       repo,
		outbox:     outbox,
		aiProvider: aiProvider,
		// now 统一提供 UTC 时钟，便于测试替换与时间断言。
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}
