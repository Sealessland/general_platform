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
	clearSelectedCartItems := len(input.Items) == 0
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
	event := s.appendOrderCreatedEvent(saved, actor, now)
	s.recordOrderCreateBehavior(saved, actor, now)
	if clearSelectedCartItems {
		_ = s.repo.DeleteSelectedCartItems(actor.UserID)
	}
	view := freshCreatedOrderView(saved, event, locks)
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
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusCreated), string(orderdomain.StatusPaid), func(o *domain.Order) error {
		o.PaidAt = &now
		o.UpdatedAt = now
		return nil
	}, func(tx OrderTx, o domain.Order) error {
		for _, lock := range tx.ListInventoryLocksByOrder(o.ID) {
			sku, ok := tx.GetSKU(lock.SKUID)
			if !ok {
				return newError(ErrorNotFound, "sku not found for inventory lock")
			}
			sku.Stock -= lock.Quantity
			sku.LockedStock -= lock.Quantity
			if sku.Stock < 0 || sku.LockedStock < 0 {
				return newError(ErrorConflict, "inventory underflow detected")
			}
			if _, err := tx.SaveSKU(sku); err != nil {
				return err
			}
			lock.Status = domain.InventoryLockStatusConfirmed
			lock.ConfirmedAt = &now
			if err := tx.UpdateInventoryLock(lock); err != nil {
				return err
			}
		}
		_, _ = tx.AppendOrderEvent(domain.OrderEvent{
			OrderID:      o.ID,
			FromStatus:   string(orderdomain.StatusCreated),
			ToStatus:     string(orderdomain.StatusPaid),
			EventType:    "ORDER_PAID",
			OperatorID:   actor.UserID,
			OperatorRole: actor.Role,
			Remark:       "payment simulated",
			CreatedAt:    now,
		})
		return nil
	})
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && payAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
	}
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
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusCreated), string(orderdomain.StatusCancelled), func(o *domain.Order) error {
		o.CancelledAt = &now
		o.UpdatedAt = now
		return nil
	}, func(tx OrderTx, o domain.Order) error {
		if err := s.releaseInventory(tx, o.ID, true); err != nil {
			return err
		}
		_, _ = tx.AppendOrderEvent(domain.OrderEvent{
			OrderID:      o.ID,
			FromStatus:   string(orderdomain.StatusCreated),
			ToStatus:     string(orderdomain.StatusCancelled),
			EventType:    "ORDER_CANCELLED",
			OperatorID:   actor.UserID,
			OperatorRole: actor.Role,
			Remark:       "consumer cancelled before payment",
			CreatedAt:    now,
		})
		return nil
	})
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && cancelAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
	}
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
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusShipped), string(orderdomain.StatusFinished), func(o *domain.Order) error {
		o.FinishedAt = &now
		o.UpdatedAt = now
		return nil
	}, nil)
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && finishAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
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
	saved, err := s.repo.UpdateOrderStatus(order.ID, string(prevStatus), string(orderdomain.StatusRefunding), func(o *domain.Order) error {
		o.UpdatedAt = now
		return nil
	}, nil)
	if err != nil {
		current, ok := s.repo.GetOrder(order.ID)
		if ok && refundAlreadyApplied(current) {
			return s.currentOrderView(current)
		}
		return nil, newError(ErrorConflict, err.Error())
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
