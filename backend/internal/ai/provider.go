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

type AIProvider interface {
	GenerateSellingPoints(ctx context.Context, req SellingPointRequest) (*SellingPointResult, error)
	GenerateBusinessReview(ctx context.Context, req BusinessReviewRequest) (*BusinessReviewResult, error)
}
