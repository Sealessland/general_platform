package application

import (
	"context"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) ListNotes(ctx context.Context) ([]NoteSummary, error) {
	_ = ctx
	notes := s.repo.ListNotes()
	out := make([]NoteSummary, 0, len(notes))
	for _, note := range notes {
		out = append(out, s.toNoteSummary(note))
	}
	return out, nil
}

func (s *Service) GetNote(ctx context.Context, noteID int64, actor *Actor) (*NoteDetail, error) {
	_ = ctx
	note, ok := s.repo.GetNote(noteID)
	if !ok {
		return nil, newError(ErrorNotFound, "note not found")
	}
	note.ViewCount++
	note.UpdatedAt = s.now()
	if err := s.repo.UpdateNote(note); err != nil {
		return nil, err
	}
	if actor != nil {
		productID := int64(0)
		if len(note.ProductIDs) > 0 {
			productID = note.ProductIDs[0]
		}
		_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
			UserID:     actor.UserID,
			EventType:  domain.BehaviorNoteView,
			NoteID:     note.ID,
			ProductID:  productID,
			MerchantID: s.primaryMerchantID(note.ProductIDs),
			CreatedAt:  s.now(),
		})
	}
	view := s.toNoteDetail(note)
	return &view, nil
}

func (s *Service) ListProducts(ctx context.Context) ([]ProductCard, error) {
	_ = ctx
	products := s.repo.ListProducts()
	out := make([]ProductCard, 0, len(products))
	for _, product := range products {
		if product.Status != domain.ProductStatusOnline {
			continue
		}
		out = append(out, s.toProductCard(product))
	}
	return out, nil
}

func (s *Service) GetProduct(ctx context.Context, productID int64, actor *Actor) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if actor != nil {
		_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
			UserID:     actor.UserID,
			EventType:  domain.BehaviorProductClick,
			ProductID:  product.ID,
			MerchantID: product.MerchantID,
			CreatedAt:  s.now(),
		})
	}
	view := s.toProductDetail(product)
	return &view, nil
}

func (s *Service) ListProductSKUs(ctx context.Context, productID int64) ([]SKUView, error) {
	_ = ctx
	if _, ok := s.repo.GetProduct(productID); !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	skus := s.repo.ListSKUsByProduct(productID)
	out := make([]SKUView, 0, len(skus))
	for _, sku := range skus {
		out = append(out, s.toSKUView(sku))
	}
	return out, nil
}

func (s *Service) primaryMerchantID(productIDs []int64) int64 {
	for _, productID := range productIDs {
		if product, ok := s.repo.GetProduct(productID); ok {
			return product.MerchantID
		}
	}
	return 0
}
