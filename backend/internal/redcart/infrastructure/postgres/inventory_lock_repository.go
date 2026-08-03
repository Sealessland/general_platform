package postgres

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// ListInventoryLocksByOrder 返回订单的全部库存锁，按 ID 升序。
func (r *Repository) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	return listInventoryLocksByOrder(r.db, orderID)
}

// listInventoryLocksByOrder / updateInventoryLock 是仓储与订单事务（pgOrderTx）
// 共用的实现，保证库存锁的查询与更新在普通与事务路径下行为一致。
func listInventoryLocksByOrder(q dbQuerier, orderID int64) []domain.InventoryLock {
	rows, err := q.Query(`SELECT id, order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at FROM inventory_locks WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.InventoryLock, 0)
	for rows.Next() {
		lock, err := scanInventoryLock(rows)
		if err != nil {
			return out
		}
		out = append(out, lock)
	}
	return out
}

// SaveInventoryLock 新增一条库存锁并返回带 ID 与时间戳的锁记录。
func (r *Repository) SaveInventoryLock(lock domain.InventoryLock) (domain.InventoryLock, error) {
	err := r.db.QueryRow(
		`INSERT INTO inventory_locks (order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP),COALESCE($9, CURRENT_TIMESTAMP))
		RETURNING id, created_at, updated_at`,
		lock.OrderID, lock.SKUID, lock.Quantity, lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, timeToSQL(lock.CreatedAt), timeToSQL(lock.UpdatedAt),
	).Scan(&lock.ID, &lock.CreatedAt, &lock.UpdatedAt)
	if err != nil {
		return domain.InventoryLock{}, err
	}
	return lock, nil
}

// UpdateInventoryLock 更新库存锁的状态与时间字段。
func (r *Repository) UpdateInventoryLock(lock domain.InventoryLock) error {
	return updateInventoryLock(r.db, lock)
}

// updateInventoryLock 按 ID 更新库存锁状态字段；与 listInventoryLocksByOrder 一样供事务路径复用。
func updateInventoryLock(q dbQuerier, lock domain.InventoryLock) error {
	_, err := q.Exec(
		`UPDATE inventory_locks SET status = $1, locked_at = $2, confirmed_at = $3, released_at = $4 WHERE id = $5`,
		lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, lock.ID,
	)
	return err
}
