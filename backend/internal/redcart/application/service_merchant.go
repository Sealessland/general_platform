package application

import (
	"context"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/event"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// MerchantListProducts 分页列出当前商家的商品（含 SKU 明细），仅限商家角色。
func (s *Service) MerchantListProducts(ctx context.Context, actor Actor, limit, offset int) ([]ProductDetail, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	products := s.repo.ListProducts(0, 0)
	out := make([]ProductDetail, 0)
	for _, product := range products {
		if product.MerchantID != actor.MerchantID {
			continue
		}
		out = append(out, s.toProductDetail(product))
	}
	return paginateProductDetails(out, limit, offset), nil
}

func paginateProductDetails(items []ProductDetail, limit, offset int) []ProductDetail {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []ProductDetail{}
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end]
}

// MerchantCreateProduct 创建商品：仅限商家角色，标题必填，初始状态为草稿。
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

// MerchantUpdateProduct 更新商品基础信息；只能操作属于自己的商品。
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

// MerchantCreateSKU 为商品新增 SKU：价格必须为正、库存非负，未指定状态时默认启用。
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

// MerchantUpdateSKU 局部更新 SKU 字段（仅更新入参中非零/非空的字段）。
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

// MerchantSetProductStatus 上下架商品（如 draft/online/offline）。
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

// MerchantListOrders 分页列出当前商家的订单（含明细/事件/库存锁定），仅限商家角色。
func (s *Service) MerchantListOrders(ctx context.Context, actor Actor, limit, offset int) ([]OrderView, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	orders := s.repo.ListOrdersByMerchant(actor.MerchantID, limit, offset)
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

// MerchantShipOrder 发货：仅允许从已支付流转，重复发货幂等返回当前视图。
func (s *Service) MerchantShipOrder(ctx context.Context, actor Actor, orderID int64, input MerchantOrderShipInput) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if shipAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusShipped); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	// 状态流转事务内的变更回调：记录发货时间戳。
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusPaid), string(orderdomain.StatusShipped), func(o *domain.Order) error {
		o.ShippedAt = &now
		o.UpdatedAt = now
		return nil
	}, nil)
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && shipAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
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
	s.appendOrderEventToOutboxAsync(saved, event.TypeOrderShipped, actor.UserID, actor.Role, strings.TrimSpace(input.LogisticsNo), now)
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// MerchantApproveRefund 商家审批退款：事务内将已确认的库存返还（Stock += Quantity），
// 幂等分支直接返回当前视图；重复审批由 refundApprovalAlreadyApplied 守卫拦截。
func (s *Service) MerchantApproveRefund(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if refundApprovalAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusRefunded); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	// 状态流转事务内的变更回调：仅更新时间戳。
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusRefunding), string(orderdomain.StatusRefunded), func(o *domain.Order) error {
		o.UpdatedAt = now
		return nil
		// sideEffect：事务内返还已确认的库存，并写入退款事件与 outbox，与状态变更原子提交。
	}, func(tx OrderTx, o domain.Order) error {
		if err := s.releaseInventory(tx, o.ID, false); err != nil {
			return err
		}
		_, _ = tx.AppendOrderEvent(domain.OrderEvent{
			OrderID:      o.ID,
			FromStatus:   string(orderdomain.StatusRefunding),
			ToStatus:     string(orderdomain.StatusRefunded),
			EventType:    "ORDER_REFUNDED",
			OperatorID:   actor.UserID,
			OperatorRole: actor.Role,
			Remark:       "merchant approved refund",
			CreatedAt:    now,
		})
		s.appendOrderEventToOutbox(tx, o, event.TypeOrderRefunded, actor.UserID, actor.Role, "merchant approved refund", now)
		return nil
	})
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && refundApprovalAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
	}
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
