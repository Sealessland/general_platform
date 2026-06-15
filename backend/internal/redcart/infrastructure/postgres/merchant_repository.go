package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) CreateMerchant(merchant domain.Merchant) (domain.Merchant, error) {
	query := `
INSERT INTO merchants (user_id, name, description, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, COALESCE($5, CURRENT_TIMESTAMP), COALESCE($6, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
	if err := r.db.QueryRow(
		query,
		merchant.UserID,
		merchant.Name,
		merchant.Description,
		merchant.Status,
		nullTime(merchant.CreatedAt),
		nullTime(merchant.UpdatedAt),
	).Scan(&merchant.ID, &merchant.CreatedAt, &merchant.UpdatedAt); err != nil {
		return domain.Merchant{}, err
	}
	return merchant, nil
}

func (r *Repository) GetMerchant(id int64) (domain.Merchant, bool) {
	merchant, err := r.queryMerchant(`SELECT id, user_id, name, description, status, created_at, updated_at FROM merchants WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return domain.Merchant{}, false
	}
	return merchant, err == nil
}

func (r *Repository) GetMerchantByUserID(userID int64) (domain.Merchant, bool) {
	merchant, err := r.queryMerchant(`SELECT id, user_id, name, description, status, created_at, updated_at FROM merchants WHERE user_id = $1`, userID)
	if err == sql.ErrNoRows {
		return domain.Merchant{}, false
	}
	return merchant, err == nil
}
