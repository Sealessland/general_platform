package application

import (
	"context"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) GetCart(ctx context.Context, actor Actor) (*CartView, error) {
	_ = ctx
	items := s.repo.ListCartItems(actor.UserID)
	view := s.buildCartView(items)
	return &view, nil
}

func (s *Service) AddCartItem(ctx context.Context, actor Actor, input CartItemInput) (*CartItemView, error) {
	_ = ctx
	if input.Quantity <= 0 {
		return nil, newError(ErrorInvalidArgument, "quantity must be positive")
	}
	sku, ok := s.repo.GetSKU(input.SKUID)
	if !ok {
		return nil, newError(ErrorNotFound, "sku not found")
	}
	product, ok := s.repo.GetProduct(sku.ProductID)
	if !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if product.Status != domain.ProductStatusOnline {
		return nil, newError(ErrorConflict, "product is not online")
	}
	for _, item := range s.repo.ListCartItems(actor.UserID) {
		if item.SKUID == input.SKUID {
			item.Quantity += input.Quantity
			item.UpdatedAt = s.now()
			saved, err := s.repo.SaveCartItem(item)
			if err != nil {
				return nil, err
			}
			_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
				UserID:     actor.UserID,
				EventType:  domain.BehaviorAddToCart,
				ProductID:  product.ID,
				SKUID:      sku.ID,
				MerchantID: product.MerchantID,
				CreatedAt:  s.now(),
			})
			view := s.toCartItemView(saved)
			return &view, nil
		}
	}
	item := domain.CartItem{
		UserID:    actor.UserID,
		ProductID: product.ID,
		SKUID:     sku.ID,
		Quantity:  input.Quantity,
		Selected:  true,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	}
	saved, err := s.repo.SaveCartItem(item)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorAddToCart,
		ProductID:  product.ID,
		SKUID:      sku.ID,
		MerchantID: product.MerchantID,
		CreatedAt:  s.now(),
	})
	view := s.toCartItemView(saved)
	return &view, nil
}

func (s *Service) UpdateCartItem(ctx context.Context, actor Actor, itemID int64, input CartItemUpdateInput) (*CartItemView, error) {
	_ = ctx
	item, ok := s.repo.GetCartItem(actor.UserID, itemID)
	if !ok {
		return nil, newError(ErrorNotFound, "cart item not found")
	}
	if input.Quantity > 0 {
		item.Quantity = input.Quantity
	}
	if input.Selected != nil {
		item.Selected = *input.Selected
	}
	item.UpdatedAt = s.now()
	saved, err := s.repo.SaveCartItem(item)
	if err != nil {
		return nil, err
	}
	view := s.toCartItemView(saved)
	return &view, nil
}

func (s *Service) DeleteCartItem(ctx context.Context, actor Actor, itemID int64) error {
	_ = ctx
	if err := s.repo.DeleteCartItem(actor.UserID, itemID); err != nil {
		return newError(ErrorNotFound, "cart item not found")
	}
	return nil
}

func (s *Service) buildCartView(items []domain.CartItem) CartView {
	out := CartView{Items: make([]CartItemView, 0, len(items))}
	for _, item := range items {
		view := s.toCartItemView(item)
		out.Items = append(out.Items, view)
		if view.Selected {
			out.SelectedItemCount++
			out.SelectedQuantity += view.Quantity
			out.SelectedAmountCent += int64(view.Quantity) * view.PriceCent
		}
	}
	return out
}

func (s *Service) toCartItemView(item domain.CartItem) CartItemView {
	product, _ := s.repo.GetProduct(item.ProductID)
	sku, _ := s.repo.GetSKU(item.SKUID)
	return CartItemView{
		ID:            item.ID,
		ProductID:     item.ProductID,
		ProductTitle:  product.Title,
		CoverURL:      product.CoverURL,
		SKUID:         item.SKUID,
		SKUName:       sku.SKUName,
		PriceCent:     sku.PriceCent,
		Quantity:      item.Quantity,
		Selected:      item.Selected,
		Stock:         sku.Stock - sku.LockedStock,
		Status:        product.Status,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
	}
}
