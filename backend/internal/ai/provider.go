package ai

import "context"

type SellingPointRequest struct {
	ProductName string
	Audience    string
	Attributes  []string
	Reviews     []string
}

type SellingPointResult struct {
	Points []string
}

type BusinessReviewRequest struct {
	WindowDays int
	GMV        int64
	RefundRate float64
}

type BusinessReviewResult struct {
	Diagnosis string
	NextSteps []string
}

type A2UISurfaceRequest struct {
	SurfaceID   string
	UserIntent  string
	ContextJSON string
}

type A2UISurfaceResult struct {
	SurfaceID string
	A2UIJSON  string
}

type AIProvider interface {
	GenerateSellingPoints(ctx context.Context, req SellingPointRequest) (*SellingPointResult, error)
	GenerateBusinessReview(ctx context.Context, req BusinessReviewRequest) (*BusinessReviewResult, error)
	GenerateA2UISurface(ctx context.Context, req A2UISurfaceRequest) (*A2UISurfaceResult, error)
}
