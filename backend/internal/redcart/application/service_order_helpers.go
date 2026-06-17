package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

type checkoutLine struct {
	product  domain.Product
	sku      domain.SKU
	quantity int
}

func (s *Service) normalizeCheckoutLines(actor Actor, items []OrderLineInput) ([]checkoutLine, error) {
	lines := items
	if len(lines) == 0 {
		for _, item := range s.repo.ListCartItems(actor.UserID) {
			if item.Selected {
				lines = append(lines, OrderLineInput{SKUID: item.SKUID, Quantity: item.Quantity})
			}
		}
	}
	if len(lines) == 0 {
		return nil, newError(ErrorInvalidArgument, "checkout items are required")
	}
	out := make([]checkoutLine, 0, len(lines))
	var merchantID int64
	for _, line := range lines {
		if line.Quantity <= 0 {
			return nil, newError(ErrorInvalidArgument, "quantity must be positive")
		}
		sku, ok := s.repo.GetSKU(line.SKUID)
		if !ok {
			return nil, newError(ErrorNotFound, "sku not found")
		}
		product, ok := s.repo.GetProduct(sku.ProductID)
		if !ok {
			return nil, newError(ErrorNotFound, "product not found")
		}
		if product.Status != domain.ProductStatusOnline {
			return nil, newError(ErrorConflict, "product is offline")
		}
		if sku.Status != domain.SKUStatusActive {
			return nil, newError(ErrorConflict, "sku is inactive")
		}
		if sku.Stock-sku.LockedStock < line.Quantity {
			return nil, newError(ErrorConflict, "stock is insufficient")
		}
		if merchantID == 0 {
			merchantID = product.MerchantID
		}
		if merchantID != product.MerchantID {
			return nil, newError(ErrorInvalidArgument, "cross-merchant checkout is not supported")
		}
		out = append(out, checkoutLine{product: product, sku: sku, quantity: line.Quantity})
	}
	return out, nil
}

func buildCreatedOrder(actor Actor, idempotencyKey string, input CheckoutInput, preview OrderPreview, now time.Time) domain.Order {
	order := domain.Order{
		OrderNo:            fmt.Sprintf("RC%014d", now.UnixNano()%1e14),
		UserID:             actor.UserID,
		MerchantID:         preview.MerchantID,
		Status:             orderdomain.StatusCreated,
		TotalAmountCent:    preview.TotalAmountCent,
		PayAmountCent:      preview.PayAmountCent,
		DiscountAmountCent: preview.DiscountAmountCent,
		IdempotencyKey:     idempotencyKey,
		ReceiverName:       strings.TrimSpace(input.ReceiverName),
		ReceiverPhone:      strings.TrimSpace(input.ReceiverPhone),
		ReceiverAddress:    strings.TrimSpace(input.ReceiverAddress),
		CreatedAt:          now,
		UpdatedAt:          now,
		Items:              make([]domain.OrderItem, 0, len(preview.Items)),
	}
	for _, item := range preview.Items {
		order.Items = append(order.Items, domain.OrderItem{
			ProductID:            item.ProductID,
			SKUID:                item.SKUID,
			ProductTitleSnapshot: item.ProductTitle,
			SKUNameSnapshot:      item.SKUName,
			PriceCentSnapshot:    item.PriceCent,
			Quantity:             item.Quantity,
			TotalAmountCent:      item.TotalAmountCent,
			CreatedAt:            now,
			UpdatedAt:            now,
		})
	}
	return order
}

func buildInventoryLocks(items []domain.OrderItem, now time.Time) []domain.InventoryLock {
	locks := make([]domain.InventoryLock, 0, len(items))
	for _, item := range items {
		locks = append(locks, domain.InventoryLock{
			SKUID:     item.SKUID,
			Quantity:  item.Quantity,
			Status:    domain.InventoryLockStatusLocked,
			LockedAt:  now,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return locks
}

func (s *Service) appendOrderCreatedEvent(order domain.Order, actor Actor, now time.Time) domain.OrderEvent {
	event, _ := s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      order.ID,
		FromStatus:   "",
		ToStatus:     string(orderdomain.StatusCreated),
		EventType:    "ORDER_CREATED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "order created",
		CreatedAt:    now,
	})
	return event
}

func (s *Service) recordOrderCreateBehavior(order domain.Order, actor Actor, now time.Time) {
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderCreate,
		OrderID:    order.ID,
		MerchantID: order.MerchantID,
		CreatedAt:  now,
	})
}

func freshCreatedOrderView(order domain.Order, event domain.OrderEvent, locks []domain.InventoryLock) OrderView {
	view := OrderView{
		ID:                 order.ID,
		OrderNo:            order.OrderNo,
		UserID:             order.UserID,
		MerchantID:         order.MerchantID,
		Status:             string(order.Status),
		TotalAmountCent:    order.TotalAmountCent,
		PayAmountCent:      order.PayAmountCent,
		DiscountAmountCent: order.DiscountAmountCent,
		ReceiverName:       order.ReceiverName,
		ReceiverPhone:      order.ReceiverPhone,
		ReceiverAddress:    order.ReceiverAddress,
		PaidAt:             order.PaidAt,
		CancelledAt:        order.CancelledAt,
		ShippedAt:          order.ShippedAt,
		FinishedAt:         order.FinishedAt,
		CreatedAt:          order.CreatedAt,
		UpdatedAt:          order.UpdatedAt,
		Items:              make([]OrderItemView, 0, len(order.Items)),
		Events:             make([]OrderEventView, 0, 1),
		InventoryLocks:     make([]InventoryLockView, 0, len(locks)),
	}
	for _, item := range order.Items {
		view.Items = append(view.Items, OrderItemView{
			ID:              item.ID,
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			ProductTitle:    item.ProductTitleSnapshot,
			SKUName:         item.SKUNameSnapshot,
			PriceCent:       item.PriceCentSnapshot,
			Quantity:        item.Quantity,
			TotalAmountCent: item.TotalAmountCent,
		})
	}
	view.Events = append(view.Events, OrderEventView{
		ID:           event.ID,
		FromStatus:   event.FromStatus,
		ToStatus:     event.ToStatus,
		EventType:    event.EventType,
		OperatorID:   event.OperatorID,
		OperatorRole: event.OperatorRole,
		Remark:       event.Remark,
		CreatedAt:    event.CreatedAt,
	})
	for _, lock := range locks {
		view.InventoryLocks = append(view.InventoryLocks, InventoryLockView{
			ID:          lock.ID,
			SKUID:       lock.SKUID,
			Quantity:    lock.Quantity,
			Status:      lock.Status,
			LockedAt:    lock.LockedAt,
			ConfirmedAt: lock.ConfirmedAt,
			ReleasedAt:  lock.ReleasedAt,
		})
	}
	return view
}

func (s *Service) buildOrderPreview(lines []checkoutLine) (*OrderPreview, error) {
	if len(lines) == 0 {
		return nil, newError(ErrorInvalidArgument, "checkout items are required")
	}
	preview := &OrderPreview{
		MerchantID:         lines[0].product.MerchantID,
		Items:              make([]OrderItemView, 0, len(lines)),
		DiscountAmountCent: 0,
		StockOK:            true,
	}
	for _, line := range lines {
		total := int64(line.quantity) * line.sku.PriceCent
		preview.Items = append(preview.Items, OrderItemView{
			ProductID:       line.product.ID,
			SKUID:           line.sku.ID,
			ProductTitle:    line.product.Title,
			SKUName:         line.sku.SKUName,
			PriceCent:       line.sku.PriceCent,
			Quantity:        line.quantity,
			TotalAmountCent: total,
		})
		preview.TotalAmountCent += total
	}
	preview.PayAmountCent = preview.TotalAmountCent - preview.DiscountAmountCent
	return preview, nil
}

func orderEventPayload(order domain.Order, operatorID int64, operatorRole string, remark string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"order_id":      order.ID,
		"order_no":      order.OrderNo,
		"user_id":       order.UserID,
		"merchant_id":   order.MerchantID,
		"status":        string(order.Status),
		"operator_id":   operatorID,
		"operator_role": operatorRole,
		"remark":        remark,
	})
}

func (s *Service) appendOrderEventToOutbox(tx OrderTx, order domain.Order, eventType event.Type, operatorID int64, operatorRole string, remark string, now time.Time) {
	payload, err := orderEventPayload(order, operatorID, operatorRole, remark)
	if err != nil {
		return
	}
	_, _ = tx.Append(context.Background(), event.Event{
		Type:       eventType,
		Topic:      eventType.Topic(),
		Payload:    payload,
		OccurredAt: now,
	})
}

func (s *Service) appendOrderEventToOutboxAsync(order domain.Order, eventType event.Type, operatorID int64, operatorRole string, remark string, now time.Time) {
	if s.outbox == nil {
		return
	}
	payload, err := orderEventPayload(order, operatorID, operatorRole, remark)
	if err != nil {
		return
	}
	_, _ = s.outbox.Append(context.Background(), event.Event{
		Type:       eventType,
		Topic:      eventType.Topic(),
		Payload:    payload,
		OccurredAt: now,
	})
}

func (s *Service) releaseInventory(tx OrderTx, orderID int64, fromLocked bool) error {
	now := s.now()
	for _, lock := range tx.ListInventoryLocksByOrder(orderID) {
		sku, ok := tx.GetSKU(lock.SKUID)
		if !ok {
			return newError(ErrorNotFound, "sku not found for inventory release")
		}
		switch {
		case fromLocked && lock.Status == domain.InventoryLockStatusLocked:
			sku.LockedStock -= lock.Quantity
		case !fromLocked && lock.Status == domain.InventoryLockStatusConfirmed:
			sku.Stock += lock.Quantity
		}
		if sku.LockedStock < 0 || sku.Stock < 0 {
			return newError(ErrorConflict, "inventory underflow detected")
		}
		if _, err := tx.SaveSKU(sku); err != nil {
			return err
		}
		lock.Status = domain.InventoryLockStatusReleased
		lock.ReleasedAt = &now
		if err := tx.UpdateInventoryLock(lock); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enrichOrderView(order domain.Order) (OrderView, error) {
	view := OrderView{
		ID:                 order.ID,
		OrderNo:            order.OrderNo,
		UserID:             order.UserID,
		MerchantID:         order.MerchantID,
		Status:             string(order.Status),
		TotalAmountCent:    order.TotalAmountCent,
		PayAmountCent:      order.PayAmountCent,
		DiscountAmountCent: order.DiscountAmountCent,
		ReceiverName:       order.ReceiverName,
		ReceiverPhone:      order.ReceiverPhone,
		ReceiverAddress:    order.ReceiverAddress,
		PaidAt:             order.PaidAt,
		CancelledAt:        order.CancelledAt,
		ShippedAt:          order.ShippedAt,
		FinishedAt:         order.FinishedAt,
		CreatedAt:          order.CreatedAt,
		UpdatedAt:          order.UpdatedAt,
		Items:              make([]OrderItemView, 0, len(order.Items)),
		Events:             make([]OrderEventView, 0),
		InventoryLocks:     make([]InventoryLockView, 0),
	}
	for _, item := range order.Items {
		view.Items = append(view.Items, OrderItemView{
			ID:              item.ID,
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			ProductTitle:    item.ProductTitleSnapshot,
			SKUName:         item.SKUNameSnapshot,
			PriceCent:       item.PriceCentSnapshot,
			Quantity:        item.Quantity,
			TotalAmountCent: item.TotalAmountCent,
		})
	}
	for _, event := range s.repo.ListOrderEvents(order.ID) {
		view.Events = append(view.Events, OrderEventView{
			ID:           event.ID,
			FromStatus:   event.FromStatus,
			ToStatus:     event.ToStatus,
			EventType:    event.EventType,
			OperatorID:   event.OperatorID,
			OperatorRole: event.OperatorRole,
			Remark:       event.Remark,
			CreatedAt:    event.CreatedAt,
		})
	}
	for _, lock := range s.repo.ListInventoryLocksByOrder(order.ID) {
		view.InventoryLocks = append(view.InventoryLocks, InventoryLockView{
			ID:          lock.ID,
			SKUID:       lock.SKUID,
			Quantity:    lock.Quantity,
			Status:      lock.Status,
			LockedAt:    lock.LockedAt,
			ConfirmedAt: lock.ConfirmedAt,
			ReleasedAt:  lock.ReleasedAt,
		})
	}
	return view, nil
}
