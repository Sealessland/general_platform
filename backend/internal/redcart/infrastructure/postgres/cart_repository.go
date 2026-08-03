package postgres

import (
	"database/sql"
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// ListCartItems 返回指定用户的购物车条目，按 ID 升序。
func (r *Repository) ListCartItems(userID int64) []domain.CartItem {
	rows, err := r.db.Query(`SELECT id, user_id, product_id, sku_id, quantity, selected, created_at, updated_at FROM cart_items WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	items := make([]domain.CartItem, 0)
	for rows.Next() {
		var item domain.CartItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.ProductID, &item.SKUID, &item.Quantity, &item.Selected, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return items
		}
		items = append(items, item)
	}
	return items
}

// GetCartItem 查询指定用户的某条购物车条目；不存在返回 (零值, false)。
func (r *Repository) GetCartItem(userID, itemID int64) (domain.CartItem, bool) {
	row := r.db.QueryRow(`SELECT id, user_id, product_id, sku_id, quantity, selected, created_at, updated_at FROM cart_items WHERE user_id = $1 AND id = $2`, userID, itemID)
	var item domain.CartItem
	if err := row.Scan(&item.ID, &item.UserID, &item.ProductID, &item.SKUID, &item.Quantity, &item.Selected, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.CartItem{}, false
	}
	return item, true
}

// SaveCartItem 新增（ID 为 0）或更新购物车条目。注意插入时 cart_id 恒为 NULL：
// 初始 schema 中的 carts 表并未实际使用，购物车条目直接归属 user_id。
func (r *Repository) SaveCartItem(item domain.CartItem) (domain.CartItem, error) {
	if item.ID == 0 {
		query := `
INSERT INTO cart_items (cart_id, user_id, product_id, sku_id, quantity, selected, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, CURRENT_TIMESTAMP), COALESCE($8, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
		if err := r.db.QueryRow(
			query, nil, item.UserID, item.ProductID, item.SKUID, item.Quantity, item.Selected, timeToSQL(item.CreatedAt), timeToSQL(item.UpdatedAt),
		).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return domain.CartItem{}, err
		}
		return item, nil
	}
	err := r.db.QueryRow(
		`UPDATE cart_items SET user_id = $1, product_id = $2, sku_id = $3, quantity = $4, selected = $5
		WHERE id = $6
		RETURNING created_at, updated_at`,
		item.UserID, item.ProductID, item.SKUID, item.Quantity, item.Selected, item.ID,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.CartItem{}, fmt.Errorf("cart item not found after update")
	}
	if err != nil {
		return domain.CartItem{}, err
	}
	return item, nil
}

// DeleteCartItem 删除指定用户的购物车条目；条目不存在时返回错误。
func (r *Repository) DeleteCartItem(userID, itemID int64) error {
	result, err := r.db.Exec(`DELETE FROM cart_items WHERE user_id = $1 AND id = $2`, userID, itemID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("cart item not found")
	}
	return nil
}

// DeleteSelectedCartItems 清空指定用户所有勾选（selected）的购物车条目。
func (r *Repository) DeleteSelectedCartItems(userID int64) error {
	_, err := r.db.Exec(`DELETE FROM cart_items WHERE user_id = $1 AND selected = TRUE`, userID)
	return err
}
