package postgres

import (
	"database/sql"
	"fmt"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
	"time"
)

func (r *Repository) FindOrderByUserAndIdempotency(userID int64, idempotencyKey string) (domain.Order, bool) {
	var orderID int64
	if err := r.db.QueryRow(`SELECT id FROM orders WHERE user_id = $1 AND idempotency_key = $2`, userID, idempotencyKey).Scan(&orderID); err != nil {
		return domain.Order{}, false
	}
	return r.GetOrder(orderID)
}

func (r *Repository) ListOrdersByUser(userID int64) []domain.Order {
	return r.listOrders(`SELECT id, order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at FROM orders WHERE user_id = $1 ORDER BY id`, userID)
}

func (r *Repository) ListOrdersByMerchant(merchantID int64) []domain.Order {
	return r.listOrders(`SELECT id, order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at FROM orders WHERE merchant_id = $1 ORDER BY id`, merchantID)
}

func (r *Repository) GetOrder(id int64) (domain.Order, bool) {
	row := r.db.QueryRow(`SELECT id, order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at FROM orders WHERE id = $1`, id)
	order, err := scanOrder(row)
	if err == sql.ErrNoRows {
		return domain.Order{}, false
	}
	if err != nil {
		return domain.Order{}, false
	}
	order.Items = r.loadOrderItems(order.ID)
	return order, true
}

func (r *Repository) SaveOrder(order domain.Order) (domain.Order, error) {
	if order.ID == 0 {
		tx, err := r.db.Begin()
		if err != nil {
			return domain.Order{}, err
		}
		defer tx.Rollback()

		err = tx.QueryRow(
			`INSERT INTO orders (order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,COALESCE($16, CURRENT_TIMESTAMP),COALESCE($17, CURRENT_TIMESTAMP))
			RETURNING id, created_at, updated_at`,
			order.OrderNo, order.UserID, order.MerchantID, string(order.Status), order.TotalAmountCent, order.PayAmountCent, order.DiscountAmountCent, order.IdempotencyKey,
			order.ReceiverName, order.ReceiverPhone, order.ReceiverAddress, order.PaidAt, order.CancelledAt, order.ShippedAt, order.FinishedAt,
			nullTime(order.CreatedAt), nullTime(order.UpdatedAt),
		).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return domain.Order{}, err
		}

		for i := range order.Items {
			item := &order.Items[i]
			item.OrderID = order.ID
			if err := tx.QueryRow(
				`INSERT INTO order_items (order_id, product_id, sku_id, product_title_snapshot, sku_name_snapshot, price_cent_snapshot, quantity, total_amount_cent, created_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9, CURRENT_TIMESTAMP),COALESCE($10, CURRENT_TIMESTAMP))
				RETURNING id, created_at, updated_at`,
				item.OrderID, item.ProductID, item.SKUID, item.ProductTitleSnapshot, item.SKUNameSnapshot, item.PriceCentSnapshot, item.Quantity, item.TotalAmountCent,
				nullTime(item.CreatedAt), nullTime(item.UpdatedAt),
			).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
				return domain.Order{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return domain.Order{}, err
		}
		return order, nil
	}

	err := r.db.QueryRow(
		`UPDATE orders SET order_no = $1, user_id = $2, merchant_id = $3, status = $4, total_amount_cent = $5, pay_amount_cent = $6, discount_amount_cent = $7, idempotency_key = $8, receiver_name = $9, receiver_phone = $10, receiver_address = $11, paid_at = $12, cancelled_at = $13, shipped_at = $14, finished_at = $15
		WHERE id = $16
		RETURNING created_at, updated_at`,
		order.OrderNo, order.UserID, order.MerchantID, string(order.Status), order.TotalAmountCent, order.PayAmountCent, order.DiscountAmountCent, order.IdempotencyKey,
		order.ReceiverName, order.ReceiverPhone, order.ReceiverAddress, order.PaidAt, order.CancelledAt, order.ShippedAt, order.FinishedAt, order.ID,
	).Scan(&order.CreatedAt, &order.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.Order{}, fmt.Errorf("order %d not found", order.ID)
	}
	if err != nil {
		return domain.Order{}, err
	}
	if len(order.Items) == 0 {
		order.Items = r.loadOrderItems(order.ID)
	}
	return order, nil
}

func (r *Repository) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	if order.ID != 0 {
		return r.SaveOrder(order)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return domain.Order{}, err
	}
	defer tx.Rollback()

	orderedLocks := locks
	if len(locks) > 1 {
		orderedLocks = append([]domain.InventoryLock(nil), locks...)
		sort.Slice(orderedLocks, func(i, j int) bool { return orderedLocks[i].SKUID < orderedLocks[j].SKUID })
	}
	for _, lock := range orderedLocks {
		result, err := tx.Exec(
			`UPDATE product_skus
			SET locked_stock = locked_stock + $1
			WHERE id = $2 AND stock - locked_stock >= $1`,
			lock.Quantity, lock.SKUID,
		)
		if err != nil {
			return domain.Order{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return domain.Order{}, err
		}
		if affected == 0 {
			return domain.Order{}, application.ErrInsufficientStock
		}
	}

	err = tx.QueryRow(
		`INSERT INTO orders (order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,COALESCE($16, CURRENT_TIMESTAMP),COALESCE($17, CURRENT_TIMESTAMP))
		RETURNING id, created_at, updated_at`,
		order.OrderNo, order.UserID, order.MerchantID, string(order.Status), order.TotalAmountCent, order.PayAmountCent, order.DiscountAmountCent, order.IdempotencyKey,
		order.ReceiverName, order.ReceiverPhone, order.ReceiverAddress, order.PaidAt, order.CancelledAt, order.ShippedAt, order.FinishedAt,
		nullTime(order.CreatedAt), nullTime(order.UpdatedAt),
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return domain.Order{}, err
	}

	for i := range order.Items {
		item := &order.Items[i]
		item.OrderID = order.ID
		if err := tx.QueryRow(
			`INSERT INTO order_items (order_id, product_id, sku_id, product_title_snapshot, sku_name_snapshot, price_cent_snapshot, quantity, total_amount_cent, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9, CURRENT_TIMESTAMP),COALESCE($10, CURRENT_TIMESTAMP))
			RETURNING id, created_at, updated_at`,
			item.OrderID, item.ProductID, item.SKUID, item.ProductTitleSnapshot, item.SKUNameSnapshot, item.PriceCentSnapshot, item.Quantity, item.TotalAmountCent,
			nullTime(item.CreatedAt), nullTime(item.UpdatedAt),
		).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return domain.Order{}, err
		}
	}

	for i := range locks {
		lock := &locks[i]
		if err := tx.QueryRow(
			`INSERT INTO inventory_locks (order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP),COALESCE($9, CURRENT_TIMESTAMP))
			RETURNING id, created_at, updated_at`,
			order.ID, lock.SKUID, lock.Quantity, lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, nullTime(lock.CreatedAt), nullTime(lock.UpdatedAt),
		).Scan(&lock.ID, &lock.CreatedAt, &lock.UpdatedAt); err != nil {
			return domain.Order{}, err
		}
		lock.OrderID = order.ID
	}

	if err := tx.Commit(); err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

type pgOrderTx struct {
	tx *gormTx
}

func (t *pgOrderTx) GetSKU(id int64) (domain.SKU, bool) {
	return getSKU(t.tx, id)
}

func (t *pgOrderTx) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	return saveSKU(t.tx, sku)
}

func (t *pgOrderTx) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	return listInventoryLocksByOrder(t.tx, orderID)
}

func (t *pgOrderTx) UpdateInventoryLock(lock domain.InventoryLock) error {
	return updateInventoryLock(t.tx, lock)
}

func (t *pgOrderTx) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	return appendOrderEvent(t.tx, event)
}

func (r *Repository) UpdateOrderStatus(orderID int64, fromStatus, toStatus string, mutator func(*domain.Order) error, sideEffect func(application.OrderTx, domain.Order) error) (domain.Order, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return domain.Order{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(
		`SELECT id, order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at
		FROM orders
		WHERE id = $1
		FOR UPDATE`,
		orderID,
	)
	order, err := scanOrder(row)
	if err == sql.ErrNoRows {
		return domain.Order{}, fmt.Errorf("order %d not found", orderID)
	}
	if err != nil {
		return domain.Order{}, err
	}
	if string(order.Status) != fromStatus {
		return domain.Order{}, fmt.Errorf("order %d status is %s, expected %s", orderID, order.Status, fromStatus)
	}

	if mutator != nil {
		if err := mutator(&order); err != nil {
			return domain.Order{}, err
		}
	}
	order.Status = orderdomain.OrderStatus(toStatus)
	order.UpdatedAt = time.Now().UTC()

	result, err := tx.Exec(
		`UPDATE orders SET status = $1, total_amount_cent = $2, pay_amount_cent = $3, discount_amount_cent = $4, receiver_name = $5, receiver_phone = $6, receiver_address = $7, paid_at = $8, cancelled_at = $9, shipped_at = $10, finished_at = $11, updated_at = $12
		WHERE id = $13 AND status = $14`,
		order.Status, order.TotalAmountCent, order.PayAmountCent, order.DiscountAmountCent,
		order.ReceiverName, order.ReceiverPhone, order.ReceiverAddress,
		nullTimeValue(order.PaidAt), nullTimeValue(order.CancelledAt), nullTimeValue(order.ShippedAt), nullTimeValue(order.FinishedAt),
		order.UpdatedAt, order.ID, fromStatus,
	)
	if err != nil {
		return domain.Order{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Order{}, err
	}
	if affected == 0 {
		return domain.Order{}, fmt.Errorf("order %d was concurrently modified", orderID)
	}

	if sideEffect != nil {
		if err := sideEffect(&pgOrderTx{tx: tx}, order); err != nil {
			return domain.Order{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Order{}, err
	}
	order.Items = r.loadOrderItems(order.ID)
	return order, nil
}

func nullTimeValue(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func (r *Repository) listOrders(query string, arg int64) []domain.Order {
	rows, err := r.db.Query(query, arg)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.Order, 0)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return out
		}
		order.Items = r.loadOrderItems(order.ID)
		out = append(out, order)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) loadOrderItems(orderID int64) []domain.OrderItem {
	rows, err := r.db.Query(`SELECT id, order_id, product_id, sku_id, product_title_snapshot, sku_name_snapshot, price_cent_snapshot, quantity, total_amount_cent, created_at, updated_at FROM order_items WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.OrderItem, 0)
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.SKUID, &item.ProductTitleSnapshot, &item.SKUNameSnapshot, &item.PriceCentSnapshot, &item.Quantity, &item.TotalAmountCent, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return out
		}
		out = append(out, item)
	}
	return out
}

type orderScanner interface {
	Scan(dest ...any) error
}

func scanOrder(scanner orderScanner) (domain.Order, error) {
	var order domain.Order
	var status string
	var paidAt, cancelledAt, shippedAt, finishedAt sql.NullTime
	err := scanner.Scan(
		&order.ID, &order.OrderNo, &order.UserID, &order.MerchantID, &status,
		&order.TotalAmountCent, &order.PayAmountCent, &order.DiscountAmountCent, &order.IdempotencyKey,
		&order.ReceiverName, &order.ReceiverPhone, &order.ReceiverAddress,
		&paidAt, &cancelledAt, &shippedAt, &finishedAt, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return domain.Order{}, err
	}
	order.Status = orderdomain.OrderStatus(status)
	order.PaidAt = nullTimePtr(paidAt)
	order.CancelledAt = nullTimePtr(cancelledAt)
	order.ShippedAt = nullTimePtr(shippedAt)
	order.FinishedAt = nullTimePtr(finishedAt)
	return order, nil
}

type inventoryLockScanner interface {
	Scan(dest ...any) error
}

func scanInventoryLock(scanner inventoryLockScanner) (domain.InventoryLock, error) {
	var lock domain.InventoryLock
	var confirmedAt, releasedAt sql.NullTime
	err := scanner.Scan(&lock.ID, &lock.OrderID, &lock.SKUID, &lock.Quantity, &lock.Status, &lock.LockedAt, &confirmedAt, &releasedAt, &lock.CreatedAt, &lock.UpdatedAt)
	if err != nil {
		return domain.InventoryLock{}, err
	}
	lock.ConfirmedAt = nullTimePtr(confirmedAt)
	lock.ReleasedAt = nullTimePtr(releasedAt)
	return lock, nil
}
