package application

import (
	"context"
	"errors"
	"strings"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) PreviewOrder(ctx context.Context, actor Actor, input CheckoutInput) (*OrderPreview, error) {
	_ = ctx
	lines, err := s.normalizeCheckoutLines(actor, input.Items)
	if err != nil {
		return nil, err
	}
	preview, err := s.buildOrderPreview(lines)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *Service) CreateOrder(ctx context.Context, actor Actor, idempotencyKey string, input CheckoutInput) (*OrderView, error) {
	_ = ctx
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, newError(ErrorInvalidArgument, "idempotency key is required")
	}
	if existing, ok := s.repo.FindOrderByUserAndIdempotency(actor.UserID, idempotencyKey); ok {
		view, err := s.enrichOrderView(existing)
		if err != nil {
			return nil, err
		}
		return &view, nil
	}
	lines, err := s.normalizeCheckoutLines(actor, input.Items)
	if err != nil {
		return nil, err
	}
	preview, err := s.buildOrderPreview(lines)
	if err != nil {
		return nil, err
	}
	if !preview.StockOK {
		return nil, newError(ErrorConflict, "stock is insufficient")
	}
	now := s.now()
	order := buildCreatedOrder(actor, idempotencyKey, input, *preview, now)
	locks := buildInventoryLocks(order.Items, now)
	saved, err := s.repo.SaveOrderWithInventoryLocks(order, locks)
	if err != nil {
		if errors.Is(err, ErrInsufficientStock) {
			return nil, newError(ErrorConflict, "stock is insufficient")
		}
		return nil, err
	}
	s.recordOrderCreated(saved, actor, now)
	_ = s.repo.DeleteSelectedCartItems(actor.UserID)
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) ListOrders(ctx context.Context, actor Actor) ([]OrderView, error) {
	_ = ctx
	orders := s.repo.ListOrdersByUser(actor.UserID)
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

func (s *Service) GetOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || (order.UserID != actor.UserID && order.MerchantID != actor.MerchantID) {
		return nil, newError(ErrorNotFound, "order not found")
	}
	view, err := s.enrichOrderView(order)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) PayOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if payAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusPaid); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusPaid
	order.PaidAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	for _, lock := range s.repo.ListInventoryLocksByOrder(saved.ID) {
		sku, ok := s.repo.GetSKU(lock.SKUID)
		if !ok {
			return nil, newError(ErrorNotFound, "sku not found for inventory lock")
		}
		sku.Stock -= lock.Quantity
		sku.LockedStock -= lock.Quantity
		if sku.Stock < 0 || sku.LockedStock < 0 {
			return nil, newError(ErrorConflict, "inventory underflow detected")
		}
		if _, err := s.repo.SaveSKU(sku); err != nil {
			return nil, err
		}
		lock.Status = domain.InventoryLockStatusConfirmed
		lock.ConfirmedAt = &now
		if err := s.repo.UpdateInventoryLock(lock); err != nil {
			return nil, err
		}
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusCreated),
		ToStatus:     string(orderdomain.StatusPaid),
		EventType:    "ORDER_PAID",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "payment simulated",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderPay,
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

func (s *Service) CancelOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if cancelAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusCancelled); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusCancelled
	order.CancelledAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	if err := s.releaseInventory(saved.ID, true); err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusCreated),
		ToStatus:     string(orderdomain.StatusCancelled),
		EventType:    "ORDER_CANCELLED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "consumer cancelled before payment",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderCancel,
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

func (s *Service) FinishOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if finishAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusFinished); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusFinished
	order.FinishedAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusShipped),
		ToStatus:     string(orderdomain.StatusFinished),
		EventType:    "ORDER_FINISHED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "consumer confirmed receipt",
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) RequestRefund(ctx context.Context, actor Actor, orderID int64, input RefundRequestInput) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if refundAlreadyApplied(order) {
		return s.currentOrderView(order)
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusRefunding); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	prevStatus := order.Status
	order.Status = orderdomain.StatusRefunding
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(prevStatus),
		ToStatus:     string(orderdomain.StatusRefunding),
		EventType:    "ORDER_REFUND_REQUESTED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       strings.TrimSpace(input.Reason),
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}
