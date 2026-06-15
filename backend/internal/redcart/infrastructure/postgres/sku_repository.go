package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) ListSKUsByProduct(productID int64) []domain.SKU {
	rows, err := r.db.Query(`SELECT id, product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status, created_at, updated_at FROM product_skus WHERE product_id = $1 ORDER BY id`, productID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.SKU, 0)
	for rows.Next() {
		sku, err := scanSKU(rows)
		if err != nil {
			return out
		}
		out = append(out, sku)
	}
	return out
}

func (r *Repository) GetSKU(id int64) (domain.SKU, bool) {
	return getSKU(r.db, id)
}

func getSKU(q dbQuerier, id int64) (domain.SKU, bool) {
	row := q.QueryRow(`SELECT id, product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status, created_at, updated_at FROM product_skus WHERE id = $1`, id)
	sku, err := scanSKU(row)
	if err == sql.ErrNoRows {
		return domain.SKU{}, false
	}
	return sku, err == nil
}

func (r *Repository) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	return saveSKU(r.db, sku)
}

func saveSKU(q dbQuerier, sku domain.SKU) (domain.SKU, error) {
	attrs, err := json.Marshal(sku.SKUAttrs)
	if err != nil {
		return domain.SKU{}, err
	}
	if sku.ID == 0 {
		query := `
INSERT INTO product_skus (product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, COALESCE($8, CURRENT_TIMESTAMP), COALESCE($9, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
		if err := q.QueryRow(
			query,
			sku.ProductID, sku.SKUName, string(attrs), sku.PriceCent, sku.Stock, sku.LockedStock, sku.Status,
			nullTime(sku.CreatedAt), nullTime(sku.UpdatedAt),
		).Scan(&sku.ID, &sku.CreatedAt, &sku.UpdatedAt); err != nil {
			return domain.SKU{}, err
		}
		return sku, nil
	}
	err = q.QueryRow(
		`UPDATE product_skus SET product_id = $1, sku_name = $2, sku_attrs_json = $3::jsonb, price_cent = $4, stock = $5, locked_stock = $6, status = $7
		WHERE id = $8
		RETURNING created_at, updated_at`,
		sku.ProductID, sku.SKUName, string(attrs), sku.PriceCent, sku.Stock, sku.LockedStock, sku.Status, sku.ID,
	).Scan(&sku.CreatedAt, &sku.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.SKU{}, fmt.Errorf("sku not found after update")
	}
	if err != nil {
		return domain.SKU{}, err
	}
	return sku, nil
}

type skuScanner interface {
	Scan(dest ...any) error
}

func scanSKU(scanner skuScanner) (domain.SKU, error) {
	var sku domain.SKU
	var attrs []byte
	err := scanner.Scan(&sku.ID, &sku.ProductID, &sku.SKUName, &attrs, &sku.PriceCent, &sku.Stock, &sku.LockedStock, &sku.Status, &sku.CreatedAt, &sku.UpdatedAt)
	if err != nil {
		return domain.SKU{}, err
	}
	_ = json.Unmarshal(attrs, &sku.SKUAttrs)
	return sku, nil
}
