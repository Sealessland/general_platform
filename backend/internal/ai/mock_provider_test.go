package ai

import (
	"context"
	"testing"
)

func TestMockProviderGenerateSellingPoints(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateSellingPoints(context.Background(), SellingPointRequest{
		ProductName: "Travel Makeup Bag",
		Audience:    "dorm users",
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if len(result.Points) == 0 {
		t.Fatal("expected selling points")
	}
}

func TestMockProviderRejectsEmptyProductName(t *testing.T) {
	provider := MockProvider{}
	if _, err := provider.GenerateSellingPoints(context.Background(), SellingPointRequest{}); err == nil {
		t.Fatal("expected error for empty product name")
	}
}

func TestMockProviderGenerateBusinessReview(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateBusinessReview(context.Background(), BusinessReviewRequest{WindowDays: 7})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if result.Diagnosis == "" {
		t.Fatal("expected diagnosis")
	}
}
