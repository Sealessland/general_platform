package postgres

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	return listInventoryLocksByOrder(r.db, orderID)
}

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

func (r *Repository) SaveInventoryLock(lock domain.InventoryLock) (domain.InventoryLock, error) {
	err := r.db.QueryRow(
		`INSERT INTO inventory_locks (order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP),COALESCE($9, CURRENT_TIMESTAMP))
		RETURNING id, created_at, updated_at`,
		lock.OrderID, lock.SKUID, lock.Quantity, lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, nullTime(lock.CreatedAt), nullTime(lock.UpdatedAt),
	).Scan(&lock.ID, &lock.CreatedAt, &lock.UpdatedAt)
	if err != nil {
		return domain.InventoryLock{}, err
	}
	return lock, nil
}

func (r *Repository) UpdateInventoryLock(lock domain.InventoryLock) error {
	return updateInventoryLock(r.db, lock)
}

func updateInventoryLock(q dbQuerier, lock domain.InventoryLock) error {
	_, err := q.Exec(
		`UPDATE inventory_locks SET status = $1, locked_at = $2, confirmed_at = $3, released_at = $4 WHERE id = $5`,
		lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, lock.ID,
	)
	return err
}
