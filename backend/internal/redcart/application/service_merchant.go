package application

import (
	"context"
	"strings"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) MerchantListProducts(ctx context.Context, actor Actor) ([]ProductDetail, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	products := s.repo.ListProducts()
	out := make([]ProductDetail, 0)
	for _, product := range products {
		if product.MerchantID != actor.MerchantID {
			continue
		}
		out = append(out, s.toProductDetail(product))
	}
	return out, nil
}

func (s *Service) MerchantCreateProduct(ctx context.Context, actor Actor, input MerchantProductInput) (*ProductDetail, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, newError(ErrorInvalidArgument, "title is required")
	}
	product, err := s.repo.SaveProduct(domain.Product{
		MerchantID:    actor.MerchantID,
		Title:         strings.TrimSpace(input.Title),
		Description:   strings.TrimSpace(input.Description),
		CoverURL:      strings.TrimSpace(input.CoverURL),
		CategoryID:    input.CategoryID,
		Status:        domain.ProductStatusDraft,
		SellingPoints: domain.CloneStringSlice(input.SellingPoints),
		CreatedAt:     s.now(),
		UpdatedAt:     s.now(),
	})
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(product)
	return &view, nil
}

func (s *Service) MerchantUpdateProduct(ctx context.Context, actor Actor, productID int64, input MerchantProductInput) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	product.Title = strings.TrimSpace(input.Title)
	product.Description = strings.TrimSpace(input.Description)
	product.CoverURL = strings.TrimSpace(input.CoverURL)
	product.CategoryID = input.CategoryID
	product.SellingPoints = domain.CloneStringSlice(input.SellingPoints)
	product.UpdatedAt = s.now()
	saved, err := s.repo.SaveProduct(product)
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(saved)
	return &view, nil
}

func (s *Service) MerchantCreateSKU(ctx context.Context, actor Actor, productID int64, input MerchantSKUInput) (*SKUView, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if input.PriceCent <= 0 || input.Stock < 0 {
		return nil, newError(ErrorInvalidArgument, "price and stock must be valid")
	}
	status := input.Status
	if status == "" {
		status = domain.SKUStatusActive
	}
	sku, err := s.repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     strings.TrimSpace(input.SKUName),
		SKUAttrs:    domain.CloneMap(input.SKUAttrs),
		PriceCent:   input.PriceCent,
		Stock:       input.Stock,
		LockedStock: 0,
		Status:      status,
		CreatedAt:   s.now(),
		UpdatedAt:   s.now(),
	})
	if err != nil {
		return nil, err
	}
	view := s.toSKUView(sku)
	return &view, nil
}

func (s *Service) MerchantUpdateSKU(ctx context.Context, actor Actor, skuID int64, input MerchantSKUInput) (*SKUView, error) {
	_ = ctx
	sku, ok := s.repo.GetSKU(skuID)
	if !ok {
		return nil, newError(ErrorNotFound, "sku not found")
	}
	product, ok := s.repo.GetProduct(sku.ProductID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if input.SKUName != "" {
		sku.SKUName = strings.TrimSpace(input.SKUName)
	}
	if input.SKUAttrs != nil {
		sku.SKUAttrs = domain.CloneMap(input.SKUAttrs)
	}
	if input.PriceCent > 0 {
		sku.PriceCent = input.PriceCent
	}
	if input.Stock >= 0 {
		sku.Stock = input.Stock
	}
	if input.Status != "" {
		sku.Status = input.Status
	}
	sku.UpdatedAt = s.now()
	saved, err := s.repo.SaveSKU(sku)
	if err != nil {
		return nil, err
	}
	view := s.toSKUView(saved)
	return &view, nil
}

func (s *Service) MerchantSetProductStatus(ctx context.Context, actor Actor, productID int64, status string) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	product.Status = status
	product.UpdatedAt = s.now()
	saved, err := s.repo.SaveProduct(product)
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(saved)
	return &view, nil
}

func (s *Service) MerchantListOrders(ctx context.Context, actor Actor) ([]OrderView, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	orders := s.repo.ListOrdersByMerchant(actor.MerchantID)
	out := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		view, err := s.enrichOrderView(order)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) MerchantShipOrder(ctx context.Context, actor Actor, orderID int64, input MerchantOrderShipInput) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusShipped); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusShipped
	order.ShippedAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusPaid),
		ToStatus:     string(orderdomain.StatusShipped),
		EventType:    "ORDER_SHIPPED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       strings.TrimSpace(input.LogisticsNo),
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) MerchantApproveRefund(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusRefunded); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusRefunded
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	if err := s.releaseInventory(saved.ID, false); err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusRefunding),
		ToStatus:     string(orderdomain.StatusRefunded),
		EventType:    "ORDER_REFUNDED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "merchant approved refund",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		EventType:  domain.BehaviorOrderRefund,
		OrderID:    saved.ID,
		MerchantID: saved.MerchantID,
		CreatedAt:  now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}
