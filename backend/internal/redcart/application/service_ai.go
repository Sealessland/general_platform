package application

import (
	"context"
	"fmt"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) GenerateSellingPoints(ctx context.Context, actor Actor, input SellingPointInput) (*AITaskView, error) {
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	task, err := s.repo.CreateAITask(domain.AIGenerationTask{
		UserID:     actor.UserID,
		MerchantID: actor.MerchantID,
		TaskType:   domain.TaskTypeSellingPoints,
		Input: map[string]any{
			"product_name": input.ProductName,
			"attributes":   input.Attributes,
			"target_users": input.TargetUsers,
			"price_cent":   input.PriceCent,
			"reviews":      input.Reviews,
		},
		Status:    domain.AITaskStatusPending,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.aiProvider.GenerateSellingPoints(ctx, backendai.SellingPointRequest{
		ProductName: input.ProductName,
		Audience:    input.TargetUsers,
		Attributes:  input.Attributes,
		Reviews:     input.Reviews,
	})
	if err != nil {
		task.Status = domain.AITaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateAITask(task)
		return nil, err
	}
	task.Status = domain.AITaskStatusCompleted
	task.Output = map[string]any{
		"core_points":          result.Points,
		"scenarios":            []string{"通勤补妆", "宿舍整理", "出差旅行"},
		"pain_points":          []string{"出门前找不到单品", "内容拍摄补妆慢"},
		"detail_title_suggest": fmt.Sprintf("%s｜内容电商详情页标题建议", input.ProductName),
		"note_copy_suggest":    fmt.Sprintf("围绕 %s 生成适合小红书的种草文案", input.ProductName),
	}
	_ = s.repo.UpdateAITask(task)
	view := s.toAITaskView(task)
	return &view, nil
}

func (s *Service) GenerateBusinessReview(ctx context.Context, actor Actor, input BusinessReviewInput) (*AITaskView, error) {
	funnel, err := s.DashboardFunnel(ctx, actor)
	if err != nil {
		return nil, err
	}
	summary, err := s.DashboardSummary(ctx, actor)
	if err != nil {
		return nil, err
	}
	task, err := s.repo.CreateAITask(domain.AIGenerationTask{
		UserID:     actor.UserID,
		MerchantID: actor.MerchantID,
		TaskType:   domain.TaskTypeBusinessReview,
		Input: map[string]any{
			"window_days": input.WindowDays,
			"product_id":  input.ProductID,
			"funnel":      funnel,
			"summary":     summary,
		},
		Status:    domain.AITaskStatusPending,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.aiProvider.GenerateBusinessReview(ctx, backendai.BusinessReviewRequest{
		WindowDays: input.WindowDays,
		GMV:        summary.GMVAmountCent,
		RefundRate: refundRate(summary),
	})
	if err != nil {
		task.Status = domain.AITaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateAITask(task)
		return nil, err
	}
	task.Status = domain.AITaskStatusCompleted
	task.Output = map[string]any{
		"diagnosis":        result.Diagnosis,
		"possible_reasons": []string{"商品卡点击后卖点不够集中", "加购到支付阶段信息损耗"},
		"optimization":     result.NextSteps,
		"next_experiments": []string{"调整主图与首屏卖点顺序", "按价格带拆分 SKU 对照实验"},
		"window_days":      input.WindowDays,
	}
	_ = s.repo.UpdateAITask(task)
	view := s.toAITaskView(task)
	return &view, nil
}

func (s *Service) GetAITask(ctx context.Context, actor Actor, taskID int64) (*AITaskView, error) {
	_ = ctx
	task, ok := s.repo.GetAITask(taskID)
	if !ok || !canReadAITask(actor, task) {
		return nil, newError(ErrorNotFound, "ai task not found")
	}
	view := s.toAITaskView(task)
	return &view, nil
}

func canReadAITask(actor Actor, task domain.AIGenerationTask) bool {
	if actor.Role == domain.RoleMerchant {
		return actor.MerchantID != 0 && task.MerchantID == actor.MerchantID
	}
	return task.UserID == actor.UserID
}

func (s *Service) toAITaskView(task domain.AIGenerationTask) AITaskView {
	return AITaskView{
		ID:           task.ID,
		TaskType:     task.TaskType,
		Status:       task.Status,
		Input:        task.Input,
		Output:       task.Output,
		ErrorMessage: task.ErrorMessage,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}
