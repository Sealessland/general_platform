package ai

import (
	"context"
	"strings"
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

func TestMockProviderGenerateSellingPointsWithDefaultAudience(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateSellingPoints(context.Background(), SellingPointRequest{
		ProductName: "Travel Makeup Bag",
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if result.Points[0] != "Travel Makeup Bag for target users" {
		t.Fatalf("expected default audience, got %+v", result.Points)
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

func TestMockProviderRejectsInvalidBusinessReviewWindow(t *testing.T) {
	provider := MockProvider{}
	if _, err := provider.GenerateBusinessReview(context.Background(), BusinessReviewRequest{}); err == nil {
		t.Fatal("expected error for invalid window")
	}
}

func TestMockProviderGenerateA2UISurfaceGreeting(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{
		SurfaceID:  "greeting-1",
		UserIntent: "show welcome",
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if result.SurfaceID != "greeting-1" {
		t.Fatalf("expected surface id greeting-1, got %s", result.SurfaceID)
	}
	if !strings.Contains(result.A2UIJSON, "createSurface") {
		t.Fatalf("expected createSurface in json, got %s", result.A2UIJSON)
	}
}

func TestMockProviderGenerateA2UISurfaceRejectsMissingSurfaceID(t *testing.T) {
	provider := MockProvider{}
	if _, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{UserIntent: "hi"}); err == nil {
		t.Fatal("expected error for missing surface id")
	}
}

func TestMockProviderGenerateA2UISurfaceRejectsMissingIntent(t *testing.T) {
	provider := MockProvider{}
	if _, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{SurfaceID: "s1"}); err == nil {
		t.Fatal("expected error for missing intent")
	}
}

func TestMockProviderGenerateA2UISurfaceShoppingGuide(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{
		SurfaceID:   "shop-1",
		UserIntent:  "dorm desk under 200元",
		ContextJSON: `{"budget":20000,"scene":"dorm_desk","products":[{"id":1,"title":"Lamp","cover_url":"http://example.com/lamp.jpg","min_price_cent":15000,"selling_points":["bright"]}],"related_notes":[{"id":1,"title":"Note","view_count":100,"like_count":10}]}`,
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !strings.Contains(result.A2UIJSON, "宿舍书桌改造方案") {
		t.Fatalf("expected dorm desk scene title, got %s", result.A2UIJSON)
	}
	if !strings.Contains(result.A2UIJSON, "updateDataModel") {
		t.Fatalf("expected data model update, got %s", result.A2UIJSON)
	}
}

func TestMockProviderGenerateA2UISurfaceShoppingGuideWithoutNotes(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{
		SurfaceID:   "shop-2",
		UserIntent:  "travel kit",
		ContextJSON: `{"budget":50000,"products":[{"id":2,"title":"Bag","cover_url":"http://example.com/bag.jpg","min_price_cent":49900,"selling_points":["compact"]}]}`,
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if strings.Contains(result.A2UIJSON, "notes_section") {
		t.Fatal("expected no notes section")
	}
}

func TestSceneTitle(t *testing.T) {
	if sceneTitle("dorm_desk") != "宿舍书桌改造方案" {
		t.Fatal("unexpected dorm title")
	}
	if sceneTitle("office_desk") != "通勤办公桌面方案" {
		t.Fatal("unexpected office title")
	}
	if sceneTitle("travel") != "出行收纳方案" {
		t.Fatal("unexpected travel title")
	}
	if sceneTitle("unknown") != "智能导购专题" {
		t.Fatal("unexpected default title")
	}
}

func TestMockProviderGenerateA2UISurfaceShoppingGuideNoBudget(t *testing.T) {
	provider := MockProvider{}
	result, err := provider.GenerateA2UISurface(context.Background(), A2UISurfaceRequest{
		SurfaceID:   "shop-nobudget",
		UserIntent:  "show products",
		ContextJSON: `{"budget":0,"products":[{"id":3,"title":"Pen","cover_url":"http://example.com/pen.jpg","min_price_cent":100,"selling_points":["smooth"]}]}`,
	})
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !strings.Contains(result.A2UIJSON, "预算不限") {
		t.Fatalf("expected unlimited budget text, got %s", result.A2UIJSON)
	}
}

func TestMockProviderNormalizeNotesHandlesInt64Fields(t *testing.T) {
	provider := MockProvider{}
	notes := []map[string]any{
		{"id": int64(7), "title": "Note 7", "view_count": int64(77), "like_count": int64(7)},
		{"id": float64(8), "title": "Note 8", "view_count": float64(88), "like_count": float64(8)},
	}
	out := provider.normalizeNotes(notes)
	if len(out) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(out))
	}
	if out[0].(map[string]any)["id"] != int64(7) {
		t.Fatalf("expected int64 id, got %v", out[0])
	}
}

func TestMockProviderNormalizeProductsHandlesVariants(t *testing.T) {
	provider := MockProvider{}
	products := []any{
		"not a map",
		map[string]any{
			"id":             int64(9),
			"title":          "Product 9",
			"cover_url":      "http://example.com/9.jpg",
			"min_price_cent": int64(999),
		},
		map[string]any{
			"id":             float64(10),
			"title":          "Product 10",
			"cover_url":      "http://example.com/10.jpg",
			"min_price_cent": float64(1099),
			"selling_points": []any{"point"},
		},
	}
	out := provider.normalizeProducts(products)
	if len(out) != 2 {
		t.Fatalf("expected 2 products, got %d", len(out))
	}
}
