package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) ListProducts() []domain.Product {
	rows, err := r.db.Query(`SELECT id, merchant_id, title, description, cover_url, category_id, status, selling_points, created_at, updated_at FROM products ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return out
		}
		out = append(out, product)
	}
	return out
}

func (r *Repository) GetProduct(id int64) (domain.Product, bool) {
	row := r.db.QueryRow(`SELECT id, merchant_id, title, description, cover_url, category_id, status, selling_points, created_at, updated_at FROM products WHERE id = $1`, id)
	product, err := scanProduct(row)
	if err == sql.ErrNoRows {
		return domain.Product{}, false
	}
	return product, err == nil
}

func (r *Repository) SaveProduct(product domain.Product) (domain.Product, error) {
	payload, err := json.Marshal(product.SellingPoints)
	if err != nil {
		return domain.Product{}, err
	}
	if product.ID == 0 {
		query := `
INSERT INTO products (merchant_id, title, description, cover_url, category_id, status, selling_points, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, COALESCE($8, CURRENT_TIMESTAMP), COALESCE($9, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
		if err := r.db.QueryRow(
			query,
			product.MerchantID,
			product.Title,
			product.Description,
			product.CoverURL,
			product.CategoryID,
			product.Status,
			string(payload),
			nullTime(product.CreatedAt),
			nullTime(product.UpdatedAt),
		).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt); err != nil {
			return domain.Product{}, err
		}
		return product, nil
	}
	err = r.db.QueryRow(
		`UPDATE products SET merchant_id = $1, title = $2, description = $3, cover_url = $4, category_id = $5, status = $6, selling_points = $7::jsonb
		WHERE id = $8
		RETURNING created_at, updated_at`,
		product.MerchantID, product.Title, product.Description, product.CoverURL, product.CategoryID, product.Status, string(payload), product.ID,
	).Scan(&product.CreatedAt, &product.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.Product{}, fmt.Errorf("product not found after update")
	}
	if err != nil {
		return domain.Product{}, err
	}
	return product, nil
}

type productScanner interface {
	Scan(dest ...any) error
}

func scanProduct(scanner productScanner) (domain.Product, error) {
	var product domain.Product
	var sellingPoints []byte
	err := scanner.Scan(&product.ID, &product.MerchantID, &product.Title, &product.Description, &product.CoverURL, &product.CategoryID, &product.Status, &sellingPoints, &product.CreatedAt, &product.UpdatedAt)
	if err != nil {
		return domain.Product{}, err
	}
	_ = json.Unmarshal(sellingPoints, &product.SellingPoints)
	return product, nil
}
