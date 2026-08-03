// Package ai 定义 AI 能力的领域接口与数据结构，供应用层调用；
// 具体实现（内存 Mock、gRPC 客户端等）由基础设施层提供。
package ai

import "context"

// SellingPointRequest 是"生成商品卖点"的请求：目标商品、人群、属性与用户评价。
type SellingPointRequest struct {
	ProductName string
	Audience    string
	Attributes  []string
	Reviews     []string
}

// SellingPointResult 是卖点生成结果，Points 为按重要性排序的卖点文案列表。
type SellingPointResult struct {
	Points []string
}

// BusinessReviewRequest 是"经营复盘"请求：统计窗口天数、GMV（分）与退款率。
type BusinessReviewRequest struct {
	WindowDays int
	GMV        int64
	RefundRate float64
}

// BusinessReviewResult 是经营复盘结果：诊断结论与建议行动项。
type BusinessReviewResult struct {
	Diagnosis string
	NextSteps []string
}

// A2UISurfaceRequest 是"A2UI 界面生成"请求：目标界面 ID、用户意图与上下文 JSON。
type A2UISurfaceRequest struct {
	SurfaceID   string
	UserIntent  string
	ContextJSON string
}

// A2UISurfaceResult 是 A2UI 生成结果，A2UIJSON 为多行 NDJSON 指令流
// （createSurface / updateComponents / updateDataModel 三段式协议）。
type A2UISurfaceResult struct {
	SurfaceID string
	A2UIJSON  string
}

// AIProvider 是 AI 能力的统一接口，应用层依赖该接口而非具体实现，
// 便于在 Mock 与真实 gRPC 服务之间切换。
type AIProvider interface {
	GenerateSellingPoints(ctx context.Context, req SellingPointRequest) (*SellingPointResult, error)
	GenerateBusinessReview(ctx context.Context, req BusinessReviewRequest) (*BusinessReviewResult, error)
	GenerateA2UISurface(ctx context.Context, req A2UISurfaceRequest) (*A2UISurfaceResult, error)
}
