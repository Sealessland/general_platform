package postgres

import (
	"database/sql"
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// toNullTime 把 *time.Time 指针转换为 sql.NullTime，
// 用于 UPDATE 语句向可空时间列写回值；nil 指针转换为无效 NullTime（NULL）。
func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

type orderScanner interface {
	Scan(dest ...any) error
}

// scanOrder 将查询行解码为 domain.Order，数据库可空的时间列
// （paid_at/cancelled_at 等）转换为 nil 指针。
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
	order.PaidAt = timeFromSQL(paidAt)
	order.CancelledAt = timeFromSQL(cancelledAt)
	order.ShippedAt = timeFromSQL(shippedAt)
	order.FinishedAt = timeFromSQL(finishedAt)
	return order, nil
}

type inventoryLockScanner interface {
	Scan(dest ...any) error
}

// scanInventoryLock 将查询行解码为 domain.InventoryLock，
// 确认/释放时间列可空，解码为 nil 指针。
func scanInventoryLock(scanner inventoryLockScanner) (domain.InventoryLock, error) {
	var lock domain.InventoryLock
	var confirmedAt, releasedAt sql.NullTime
	err := scanner.Scan(&lock.ID, &lock.OrderID, &lock.SKUID, &lock.Quantity, &lock.Status, &lock.LockedAt, &confirmedAt, &releasedAt, &lock.CreatedAt, &lock.UpdatedAt)
	if err != nil {
		return domain.InventoryLock{}, err
	}
	lock.ConfirmedAt = timeFromSQL(confirmedAt)
	lock.ReleasedAt = timeFromSQL(releasedAt)
	return lock, nil
}
