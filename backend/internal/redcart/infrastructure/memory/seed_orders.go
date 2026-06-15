package memory

import (
	"fmt"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) seedOrderCreated(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDC%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "CREATED",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-created-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Shanghai Xuhui District",
		CreatedAt:          base,
		UpdatedAt:          base,
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            base,
			},
		},
	})
	sku.LockedStock += quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:   order.ID,
		SKUID:     sku.ID,
		Quantity:  quantity,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  base,
		CreatedAt: base,
		UpdatedAt: base,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{
		OrderID:      order.ID,
		ToStatus:     "CREATED",
		EventType:    "ORDER_CREATED",
		OperatorID:   userID,
		OperatorRole: domain.RoleConsumer,
		Remark:       "seeded created order",
		CreatedAt:    base,
	})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     userID,
		EventType:  domain.BehaviorOrderCreate,
		ProductID:  product.ID,
		SKUID:      sku.ID,
		OrderID:    order.ID,
		MerchantID: merchantID,
		CreatedAt:  base,
	})
}

func (r *Repository) seedOrderShipped(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	paidAt := base.Add(30 * time.Minute)
	shippedAt := base.Add(90 * time.Minute)
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDS%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "SHIPPED",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-shipped-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Hangzhou Binjiang",
		PaidAt:             &paidAt,
		ShippedAt:          &shippedAt,
		CreatedAt:          base,
		UpdatedAt:          shippedAt,
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            shippedAt,
			},
		},
	})
	sku.Stock -= quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:     order.ID,
		SKUID:       sku.ID,
		Quantity:    quantity,
		Status:      domain.InventoryLockStatusConfirmed,
		LockedAt:    base,
		ConfirmedAt: &paidAt,
		CreatedAt:   base,
		UpdatedAt:   shippedAt,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, ToStatus: "CREATED", EventType: "ORDER_CREATED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded created order", CreatedAt: base})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "CREATED", ToStatus: "PAID", EventType: "ORDER_PAID", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded paid order", CreatedAt: paidAt})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "PAID", ToStatus: "SHIPPED", EventType: "ORDER_SHIPPED", OperatorID: 2, OperatorRole: domain.RoleMerchant, Remark: "seeded shipped order", CreatedAt: shippedAt})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderCreate, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: base})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderPay, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt})
}

func (r *Repository) seedOrderRefunding(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	paidAt := base.Add(20 * time.Minute)
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDR%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "REFUNDING",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-refunding-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Suzhou Industrial Park",
		PaidAt:             &paidAt,
		CreatedAt:          base,
		UpdatedAt:          paidAt.Add(2 * time.Hour),
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            paidAt,
			},
		},
	})
	sku.Stock -= quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:     order.ID,
		SKUID:       sku.ID,
		Quantity:    quantity,
		Status:      domain.InventoryLockStatusConfirmed,
		LockedAt:    base,
		ConfirmedAt: &paidAt,
		CreatedAt:   base,
		UpdatedAt:   paidAt,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, ToStatus: "CREATED", EventType: "ORDER_CREATED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded created order", CreatedAt: base})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "CREATED", ToStatus: "PAID", EventType: "ORDER_PAID", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded paid order", CreatedAt: paidAt})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "PAID", ToStatus: "REFUNDING", EventType: "ORDER_REFUND_REQUESTED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded refunding order", CreatedAt: paidAt.Add(2 * time.Hour)})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderCreate, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: base})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderPay, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderRefund, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt.Add(2 * time.Hour)})
}
