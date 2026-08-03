package application

import (
	"context"
	"errors"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/event"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// PreviewOrder 预演结算：校验商品与库存后返回金额明细和库存是否充足，不产生落库副作用。
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

// CreateOrder 创建订单：支持按幂等键去重；不传商品明细时结算购物车中已勾选项，
// 落库时同时写入库存锁定记录；若商品来源于购物车，下单成功后清空勾选项。
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
	createdEvent := s.appendOrderCreatedEvent(saved, actor, now)
	s.recordOrderCreateBehavior(saved, actor, now)
	s.appendOrderEventToOutboxAsync(saved, event.TypeOrderCreated, actor.UserID, actor.Role, "order created", now)
	if clearSelectedCartItems {
		_ = s.repo.DeleteSelectedCartItems(actor.UserID)
	}
	view := freshCreatedOrderView(saved, createdEvent, locks)
	return &view, nil
}

// ListOrders 分页返回当前消费者的订单列表。
func (s *Service) ListOrders(ctx context.Context, actor Actor, limit, offset int) ([]OrderView, error) {
	_ = ctx
	orders := s.repo.ListOrdersByUser(actor.UserID, limit, offset)
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

// GetOrder 返回订单详情；消费者或订单所属商家均可读取。
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

// PayOrder 模拟支付：事务内将锁定库存转正（扣减可用库存并确认锁定记录），
// 幂等分支直接返回当前视图；重复支付由 payAlreadyApplied 守卫拦截。
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
		s.appendOrderEventToOutbox(tx, o, event.TypeOrderPaid, actor.UserID, actor.Role, "payment simulated", now)
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

// CancelOrder 支付前取消订单：事务内释放已锁定但未扣减的库存，幂等分支直接返回当前视图。
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
		s.appendOrderEventToOutbox(tx, o, event.TypeOrderCancelled, actor.UserID, actor.Role, "consumer cancelled before payment", now)
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

// FinishOrder 消费者确认收货，将已发货订单流转为已完成；幂等分支直接返回当前视图。
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
	s.appendOrderEventToOutboxAsync(saved, event.TypeOrderFinished, actor.UserID, actor.Role, "consumer confirmed receipt", now)
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// RequestRefund 消费者申请退款：已支付/已发货订单均可发起，进入退款中状态；幂等分支返回当前视图。
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
	s.appendOrderEventToOutboxAsync(saved, event.TypeOrderRefundRequested, actor.UserID, actor.Role, strings.TrimSpace(input.Reason), now)
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}
