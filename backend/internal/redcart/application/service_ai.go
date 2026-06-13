package application

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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

func (s *Service) GenerateA2UISurface(ctx context.Context, actor Actor, input A2UISurfaceInput) (*A2UISurfaceView, error) {
	_ = actor
	enrichedContext, err := s.enrichA2UIContext(ctx, input)
	if err != nil {
		return nil, err
	}
	result, err := s.aiProvider.GenerateA2UISurface(ctx, backendai.A2UISurfaceRequest{
		SurfaceID:   input.SurfaceID,
		UserIntent:  input.UserIntent,
		ContextJSON: enrichedContext,
	})
	if err != nil {
		return nil, err
	}
	return &A2UISurfaceView{
		SurfaceID: result.SurfaceID,
		A2UIJSON:  result.A2UIJSON,
	}, nil
}

func (s *Service) enrichA2UIContext(ctx context.Context, input A2UISurfaceInput) (string, error) {
	contextMap := map[string]any{}
	if input.ContextJSON != "" {
		if err := json.Unmarshal([]byte(input.ContextJSON), &contextMap); err != nil {
			return "", newError(ErrorInvalidArgument, "invalid context_json")
		}
	}

	// Detect shopping-guide intent: intent mentions budget or context has budget/scene.
	budget := parseBudgetFromIntent(input.UserIntent)
	if b, ok := contextMap["budget"]; ok {
		switch v := b.(type) {
		case float64:
			budget = int64(v)
		case int:
			budget = int64(v)
		case int64:
			budget = v
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				budget = parsed
			}
		}
	}
	if budget <= 0 {
		// Fallback: keep original context if no shopping guide signal detected.
		if input.ContextJSON == "" {
			return "{}", nil
		}
		return input.ContextJSON, nil
	}

	products, err := s.ListProducts(ctx)
	if err != nil {
		return "", err
	}
	filtered := make([]ProductCard, 0, len(products))
	productIDs := make([]int64, 0, len(products))
	for _, product := range products {
		if product.MinPriceCent > 0 && product.MinPriceCent <= budget {
			filtered = append(filtered, product)
			productIDs = append(productIDs, product.ID)
		}
	}

	notes := s.relatedNotes(ctx, productIDs)
	batchItems := make([]map[string]any, 0, len(filtered))
	for _, product := range filtered {
		batchItems = append(batchItems, map[string]any{
			"product_id": product.ID,
			"sku_id":     product.ID, // Simplified: default SKU mapped to product id for demo.
			"title":      product.Title,
		})
	}

	contextMap["budget"] = budget
	contextMap["products"] = filtered
	contextMap["related_notes"] = toA2UINotes(notes)
	contextMap["batch_cart_items"] = batchItems
	if _, ok := contextMap["scene"]; !ok {
		contextMap["scene"] = inferSceneFromIntent(input.UserIntent)
	}

	out, err := json.Marshal(contextMap)
	if err != nil {
		return "", fmt.Errorf("marshal a2ui context: %w", err)
	}
	return string(out), nil
}

func toA2UINotes(notes []NoteSummary) []map[string]any {
	out := make([]map[string]any, 0, len(notes))
	for _, note := range notes {
		out = append(out, map[string]any{
			"id":         note.ID,
			"title":      note.Title,
			"view_count": note.ViewCount,
			"like_count": note.LikeCount,
		})
	}
	return out
}

func (s *Service) relatedNotes(ctx context.Context, productIDs []int64) []NoteSummary {
	_ = ctx
	allNotes := s.repo.ListNotes()
	wanted := make(map[int64]struct{}, len(productIDs))
	for _, id := range productIDs {
		wanted[id] = struct{}{}
	}
	out := make([]NoteSummary, 0, 4)
	for _, note := range allNotes {
		for _, pid := range note.ProductIDs {
			if _, ok := wanted[pid]; ok {
				out = append(out, s.toNoteSummary(note))
				break
			}
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

var budgetRegex = regexp.MustCompile(`(?i)(\d+)\s*(百|元|块|rmb|yuan)?`)

func parseBudgetFromIntent(intent string) int64 {
	intent = strings.ToLower(intent)
	matches := budgetRegex.FindStringSubmatch(intent)
	if len(matches) < 2 {
		return 0
	}
	num, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0
	}
	// Heuristic: "300块" => 30000 分; "300元" => 30000 分; plain "300" in budget context treated as yuan.
	if strings.Contains(intent, "百") {
		return num * 100 * 100
	}
	return num * 100
}

func inferSceneFromIntent(intent string) string {
	intent = strings.ToLower(intent)
	if strings.Contains(intent, "宿舍") {
		return "dorm_desk"
	}
	if strings.Contains(intent, "通勤") || strings.Contains(intent, "办公") {
		return "office_desk"
	}
	if strings.Contains(intent, "出行") || strings.Contains(intent, "旅行") {
		return "travel"
	}
	return "general"
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
