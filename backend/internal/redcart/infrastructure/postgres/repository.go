package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "github.com/jackc/pgx/stdlib"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

type Repository struct {
	db *sql.DB

	sessionMu sync.RWMutex
	sessions  map[string]int64
}

var _ application.Repository = (*Repository)(nil)

func NewRepository(dsn string) (*Repository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	repo := &Repository{
		db:       db,
		sessions: make(map[string]int64),
	}
	if err := repo.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.seed(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) migrate(ctx context.Context) error {
	var exists string
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(to_regclass('public.users')::text, '')`).Scan(&exists)
	if err == nil && exists == "users" {
		return nil
	}
	migrationFile, err := resolveMigrationFile()
	if err != nil {
		return err
	}
	sqlText, err := os.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, string(sqlText)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}

func resolveMigrationFile() (string, error) {
	candidates := []string{
		filepath.Join(envOrDefault("MIGRATIONS_DIR", ""), "0001_init_schema.sql"),
		filepath.Join("migrations", "0001_init_schema.sql"),
		filepath.Join("/app/migrations", "0001_init_schema.sql"),
	}
	for _, candidate := range candidates {
		if candidate == "" || candidate == "0001_init_schema.sql" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("migration file not found")
}

func (r *Repository) seed(ctx context.Context) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	seedSQL := `
INSERT INTO users (id, nickname, phone, password_hash, role) VALUES
  (1, 'Alice', '13800000001', '` + seededPasswordHash("consumer-demo") + `', 'consumer'),
  (2, 'Merchant Zoe', '13800000002', '` + seededPasswordHash("merchant-demo") + `', 'merchant')
ON CONFLICT DO NOTHING;

INSERT INTO merchants (id, user_id, name, description, status) VALUES
  (1, 2, 'RedCart Beauty Lab', 'Content-driven beauty merchant demo account', 'active')
ON CONFLICT DO NOTHING;

INSERT INTO products (id, merchant_id, title, description, cover_url, category_id, status, selling_points) VALUES
  (1, 1, 'Velvet Lip Mud Set', 'Matte lip set for commute, campus, and quick content shoots.', 'https://images.example.com/lip-mud.jpg', 101, 'online', '["Soft matte finish","Pocket-size touch-up","Daily shade bundle"]'::jsonb),
  (2, 1, 'Travel Makeup Organizer', 'Portable makeup storage for dorm, travel, and desk organization.', 'https://images.example.com/makeup-organizer.jpg', 102, 'online', '["Compartment layout","Portable and lightweight","Easy to clean"]'::jsonb)
ON CONFLICT DO NOTHING;

INSERT INTO product_skus (id, product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status) VALUES
  (1, 1, 'Cherry Set', '{"shade":"cherry","pack":"3pcs"}'::jsonb, 12900, 29, 1, 'active'),
  (2, 1, 'Rose Set', '{"shade":"rose","pack":"3pcs"}'::jsonb, 13900, 18, 1, 'active'),
  (3, 2, 'Cream White', '{"color":"cream","size":"standard"}'::jsonb, 8900, 38, 0, 'active')
ON CONFLICT DO NOTHING;

INSERT INTO notes (id, author_id, title, content, cover_url, status, view_count, like_count) VALUES
  (1, 2, '通勤妆 5 分钟出门组合', '这套唇泥和整理盒是我最近拍通勤内容最常带的组合，颜色稳、补妆快、包里不乱。', 'https://images.example.com/note-commute.jpg', 'published', 1280, 218),
  (2, 2, '宿舍桌面整理前后对比', '桌面一乱，化妆和出门效率都会掉。这个整理盒适合小桌面。', 'https://images.example.com/note-dorm.jpg', 'published', 920, 141)
ON CONFLICT DO NOTHING;

INSERT INTO note_products (id, note_id, product_id) VALUES
  (1, 1, 1),
  (2, 1, 2),
  (3, 2, 2)
ON CONFLICT DO NOTHING;

INSERT INTO cart_items (id, user_id, product_id, sku_id, quantity, selected) VALUES
  (1, 1, 1, 1, 1, true)
ON CONFLICT DO NOTHING;

INSERT INTO orders (id, order_no, user_id, merchant_id, status, total_amount_cent, pay_amount_cent, discount_amount_cent, idempotency_key, receiver_name, receiver_phone, receiver_address, paid_at, cancelled_at, shipped_at, finished_at, created_at, updated_at) VALUES
  (1, 'RCSEEDC000002', 1, 1, 'CREATED', 13900, 13900, 0, 'seed-created-2', 'Alice', '13800000001', 'Shanghai Xuhui District', NULL, NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '4 hours', CURRENT_TIMESTAMP - INTERVAL '4 hours'),
  (2, 'RCSEEDS000003', 1, 1, 'SHIPPED', 8900, 8900, 0, 'seed-shipped-3', 'Alice', '13800000001', 'Hangzhou Binjiang', CURRENT_TIMESTAMP - INTERVAL '47 hours 30 minutes', NULL, CURRENT_TIMESTAMP - INTERVAL '46 hours 30 minutes', NULL, CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '46 hours 30 minutes'),
  (3, 'RCSEEDR000001', 1, 1, 'REFUNDING', 12900, 12900, 0, 'seed-refunding-1', 'Alice', '13800000001', 'Suzhou Industrial Park', CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes', NULL, NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '24 hours', CURRENT_TIMESTAMP - INTERVAL '21 hours 40 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO order_items (id, order_id, product_id, sku_id, product_title_snapshot, sku_name_snapshot, price_cent_snapshot, quantity, total_amount_cent, created_at, updated_at) VALUES
  (1, 1, 1, 2, 'Velvet Lip Mud Set', 'Rose Set', 13900, 1, 13900, CURRENT_TIMESTAMP - INTERVAL '4 hours', CURRENT_TIMESTAMP - INTERVAL '4 hours'),
  (2, 2, 2, 3, 'Travel Makeup Organizer', 'Cream White', 8900, 1, 8900, CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '46 hours 30 minutes'),
  (3, 3, 1, 1, 'Velvet Lip Mud Set', 'Cherry Set', 12900, 1, 12900, CURRENT_TIMESTAMP - INTERVAL '24 hours', CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO order_events (id, order_id, from_status, to_status, event_type, operator_id, operator_role, remark, created_at) VALUES
  (1, 1, NULL, 'CREATED', 'ORDER_CREATED', 1, 'consumer', 'seeded created order', CURRENT_TIMESTAMP - INTERVAL '4 hours'),
  (2, 2, NULL, 'CREATED', 'ORDER_CREATED', 1, 'consumer', 'seeded created order', CURRENT_TIMESTAMP - INTERVAL '48 hours'),
  (3, 2, 'CREATED', 'PAID', 'ORDER_PAID', 1, 'consumer', 'seeded paid order', CURRENT_TIMESTAMP - INTERVAL '47 hours 30 minutes'),
  (4, 2, 'PAID', 'SHIPPED', 'ORDER_SHIPPED', 2, 'merchant', 'seeded shipped order', CURRENT_TIMESTAMP - INTERVAL '46 hours 30 minutes'),
  (5, 3, NULL, 'CREATED', 'ORDER_CREATED', 1, 'consumer', 'seeded created order', CURRENT_TIMESTAMP - INTERVAL '24 hours'),
  (6, 3, 'CREATED', 'PAID', 'ORDER_PAID', 1, 'consumer', 'seeded paid order', CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes'),
  (7, 3, 'PAID', 'REFUNDING', 'ORDER_REFUND_REQUESTED', 1, 'consumer', 'seeded refunding order', CURRENT_TIMESTAMP - INTERVAL '21 hours 40 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO inventory_locks (id, order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at) VALUES
  (1, 1, 2, 1, 'locked', CURRENT_TIMESTAMP - INTERVAL '4 hours', NULL, NULL, CURRENT_TIMESTAMP - INTERVAL '4 hours', CURRENT_TIMESTAMP - INTERVAL '4 hours'),
  (2, 2, 3, 1, 'confirmed', CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '47 hours 30 minutes', NULL, CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '46 hours 30 minutes'),
  (3, 3, 1, 1, 'confirmed', CURRENT_TIMESTAMP - INTERVAL '24 hours', CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes', NULL, CURRENT_TIMESTAMP - INTERVAL '24 hours', CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes')
ON CONFLICT DO NOTHING;

INSERT INTO behavior_events (id, user_id, event_type, note_id, product_id, sku_id, order_id, merchant_id, created_at) VALUES
  (1, 1, 'NOTE_VIEW', 1, NULL, NULL, NULL, 1, CURRENT_TIMESTAMP),
  (2, 1, 'NOTE_VIEW', 2, NULL, NULL, NULL, 1, CURRENT_TIMESTAMP),
  (3, 1, 'PRODUCT_CLICK', NULL, 1, NULL, NULL, 1, CURRENT_TIMESTAMP),
  (4, 1, 'PRODUCT_CLICK', NULL, 2, NULL, NULL, 1, CURRENT_TIMESTAMP),
  (5, 1, 'ADD_TO_CART', NULL, 1, 1, NULL, 1, CURRENT_TIMESTAMP),
  (6, 1, 'ADD_TO_CART', NULL, 2, 3, NULL, 1, CURRENT_TIMESTAMP),
  (7, 1, 'ORDER_CREATE', NULL, 1, 2, 1, 1, CURRENT_TIMESTAMP - INTERVAL '4 hours'),
  (8, 1, 'ORDER_CREATE', NULL, 2, 3, 2, 1, CURRENT_TIMESTAMP - INTERVAL '48 hours'),
  (9, 1, 'ORDER_PAY', NULL, 2, 3, 2, 1, CURRENT_TIMESTAMP - INTERVAL '47 hours 30 minutes'),
  (10, 1, 'ORDER_CREATE', NULL, 1, 1, 3, 1, CURRENT_TIMESTAMP - INTERVAL '24 hours'),
  (11, 1, 'ORDER_PAY', NULL, 1, 1, 3, 1, CURRENT_TIMESTAMP - INTERVAL '23 hours 40 minutes'),
  (12, 1, 'ORDER_REFUND', NULL, 1, 1, 3, 1, CURRENT_TIMESTAMP - INTERVAL '21 hours 40 minutes')
ON CONFLICT DO NOTHING;

SELECT setval(pg_get_serial_sequence('users', 'id'), COALESCE((SELECT MAX(id) FROM users), 1), true);
SELECT setval(pg_get_serial_sequence('merchants', 'id'), COALESCE((SELECT MAX(id) FROM merchants), 1), true);
SELECT setval(pg_get_serial_sequence('notes', 'id'), COALESCE((SELECT MAX(id) FROM notes), 1), true);
SELECT setval(pg_get_serial_sequence('note_products', 'id'), COALESCE((SELECT MAX(id) FROM note_products), 1), true);
SELECT setval(pg_get_serial_sequence('products', 'id'), COALESCE((SELECT MAX(id) FROM products), 1), true);
SELECT setval(pg_get_serial_sequence('product_skus', 'id'), COALESCE((SELECT MAX(id) FROM product_skus), 1), true);
SELECT setval(pg_get_serial_sequence('cart_items', 'id'), COALESCE((SELECT MAX(id) FROM cart_items), 1), true);
SELECT setval(pg_get_serial_sequence('orders', 'id'), COALESCE((SELECT MAX(id) FROM orders), 1), true);
SELECT setval(pg_get_serial_sequence('order_items', 'id'), COALESCE((SELECT MAX(id) FROM order_items), 1), true);
SELECT setval(pg_get_serial_sequence('order_events', 'id'), COALESCE((SELECT MAX(id) FROM order_events), 1), true);
SELECT setval(pg_get_serial_sequence('inventory_locks', 'id'), COALESCE((SELECT MAX(id) FROM inventory_locks), 1), true);
SELECT setval(pg_get_serial_sequence('behavior_events', 'id'), COALESCE((SELECT MAX(id) FROM behavior_events), 1), true);
`

	if _, err := r.db.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("seed postgres: %w", err)
	}
	return nil
}

func (r *Repository) CreateUser(user domain.User) (domain.User, error) {
	query := `
INSERT INTO users (nickname, phone, password_hash, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, COALESCE($5, CURRENT_TIMESTAMP), COALESCE($6, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
	if err := r.db.QueryRow(
		query,
		user.Nickname,
		user.Phone,
		user.PasswordHash,
		user.Role,
		nullTime(user.CreatedAt),
		nullTime(user.UpdatedAt),
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *Repository) FindUserByPhone(phone string) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE phone = $1`, phone)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

func (r *Repository) GetUser(id int64) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

func (r *Repository) SaveSession(token string, userID int64) {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.sessions[token] = userID
}

func (r *Repository) GetUserByToken(token string) (domain.User, bool) {
	r.sessionMu.RLock()
	userID, ok := r.sessions[token]
	r.sessionMu.RUnlock()
	if !ok {
		return domain.User{}, false
	}
	return r.GetUser(userID)
}

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

func (r *Repository) ListNotes() []domain.Note {
	rows, err := r.db.Query(`SELECT id, author_id, title, content, cover_url, status, view_count, like_count, created_at, updated_at FROM notes ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	notes := make([]domain.Note, 0)
	for rows.Next() {
		var note domain.Note
		if err := rows.Scan(&note.ID, &note.AuthorID, &note.Title, &note.Content, &note.CoverURL, &note.Status, &note.ViewCount, &note.LikeCount, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return notes
		}
		note.ProductIDs = r.loadNoteProductIDs(note.ID)
		notes = append(notes, note)
	}
	return notes
}

func (r *Repository) GetNote(id int64) (domain.Note, bool) {
	row := r.db.QueryRow(`SELECT id, author_id, title, content, cover_url, status, view_count, like_count, created_at, updated_at FROM notes WHERE id = $1`, id)
	var note domain.Note
	if err := row.Scan(&note.ID, &note.AuthorID, &note.Title, &note.Content, &note.CoverURL, &note.Status, &note.ViewCount, &note.LikeCount, &note.CreatedAt, &note.UpdatedAt); err != nil {
		return domain.Note{}, false
	}
	note.ProductIDs = r.loadNoteProductIDs(note.ID)
	return note, true
}

func (r *Repository) UpdateNote(note domain.Note) error {
	_, err := r.db.Exec(
		`UPDATE notes SET author_id = $1, title = $2, content = $3, cover_url = $4, status = $5, view_count = $6, like_count = $7 WHERE id = $8`,
		note.AuthorID, note.Title, note.Content, note.CoverURL, note.Status, note.ViewCount, note.LikeCount, note.ID,
	)
	return err
}

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
	_, err = r.db.Exec(
		`UPDATE products SET merchant_id = $1, title = $2, description = $3, cover_url = $4, category_id = $5, status = $6, selling_points = $7::jsonb WHERE id = $8`,
		product.MerchantID, product.Title, product.Description, product.CoverURL, product.CategoryID, product.Status, string(payload), product.ID,
	)
	if err != nil {
		return domain.Product{}, err
	}
	updated, ok := r.GetProduct(product.ID)
	if !ok {
		return domain.Product{}, fmt.Errorf("product not found after update")
	}
	return updated, nil
}

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
	row := r.db.QueryRow(`SELECT id, product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status, created_at, updated_at FROM product_skus WHERE id = $1`, id)
	sku, err := scanSKU(row)
	if err == sql.ErrNoRows {
		return domain.SKU{}, false
	}
	return sku, err == nil
}

func (r *Repository) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	attrs, err := json.Marshal(sku.SKUAttrs)
	if err != nil {
		return domain.SKU{}, err
	}
	if sku.ID == 0 {
		query := `
INSERT INTO product_skus (product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, COALESCE($8, CURRENT_TIMESTAMP), COALESCE($9, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
		if err := r.db.QueryRow(
			query,
			sku.ProductID, sku.SKUName, string(attrs), sku.PriceCent, sku.Stock, sku.LockedStock, sku.Status,
			nullTime(sku.CreatedAt), nullTime(sku.UpdatedAt),
		).Scan(&sku.ID, &sku.CreatedAt, &sku.UpdatedAt); err != nil {
			return domain.SKU{}, err
		}
		return sku, nil
	}
	_, err = r.db.Exec(
		`UPDATE product_skus SET product_id = $1, sku_name = $2, sku_attrs_json = $3::jsonb, price_cent = $4, stock = $5, locked_stock = $6, status = $7 WHERE id = $8`,
		sku.ProductID, sku.SKUName, string(attrs), sku.PriceCent, sku.Stock, sku.LockedStock, sku.Status, sku.ID,
	)
	if err != nil {
		return domain.SKU{}, err
	}
	updated, ok := r.GetSKU(sku.ID)
	if !ok {
		return domain.SKU{}, fmt.Errorf("sku not found after update")
	}
	return updated, nil
}

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

func (r *Repository) GetCartItem(userID, itemID int64) (domain.CartItem, bool) {
	row := r.db.QueryRow(`SELECT id, user_id, product_id, sku_id, quantity, selected, created_at, updated_at FROM cart_items WHERE user_id = $1 AND id = $2`, userID, itemID)
	var item domain.CartItem
	if err := row.Scan(&item.ID, &item.UserID, &item.ProductID, &item.SKUID, &item.Quantity, &item.Selected, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.CartItem{}, false
	}
	return item, true
}

func (r *Repository) SaveCartItem(item domain.CartItem) (domain.CartItem, error) {
	if item.ID == 0 {
		query := `
INSERT INTO cart_items (cart_id, user_id, product_id, sku_id, quantity, selected, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, CURRENT_TIMESTAMP), COALESCE($8, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
		if err := r.db.QueryRow(
			query, nil, item.UserID, item.ProductID, item.SKUID, item.Quantity, item.Selected, nullTime(item.CreatedAt), nullTime(item.UpdatedAt),
		).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return domain.CartItem{}, err
		}
		return item, nil
	}
	_, err := r.db.Exec(
		`UPDATE cart_items SET user_id = $1, product_id = $2, sku_id = $3, quantity = $4, selected = $5 WHERE id = $6`,
		item.UserID, item.ProductID, item.SKUID, item.Quantity, item.Selected, item.ID,
	)
	if err != nil {
		return domain.CartItem{}, err
	}
	updated, ok := r.GetCartItem(item.UserID, item.ID)
	if !ok {
		return domain.CartItem{}, fmt.Errorf("cart item not found after update")
	}
	return updated, nil
}

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

func (r *Repository) DeleteSelectedCartItems(userID int64) error {
	_, err := r.db.Exec(`DELETE FROM cart_items WHERE user_id = $1 AND selected = TRUE`, userID)
	return err
}

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

		for _, item := range order.Items {
			if _, err := tx.Exec(
				`INSERT INTO order_items (order_id, product_id, sku_id, product_title_snapshot, sku_name_snapshot, price_cent_snapshot, quantity, total_amount_cent, created_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9, CURRENT_TIMESTAMP),COALESCE($10, CURRENT_TIMESTAMP))`,
				order.ID, item.ProductID, item.SKUID, item.ProductTitleSnapshot, item.SKUNameSnapshot, item.PriceCentSnapshot, item.Quantity, item.TotalAmountCent,
				nullTime(item.CreatedAt), nullTime(item.UpdatedAt),
			); err != nil {
				return domain.Order{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return domain.Order{}, err
		}
		return mustGetOrder(r, order.ID)
	}

	_, err := r.db.Exec(
		`UPDATE orders SET order_no = $1, user_id = $2, merchant_id = $3, status = $4, total_amount_cent = $5, pay_amount_cent = $6, discount_amount_cent = $7, idempotency_key = $8, receiver_name = $9, receiver_phone = $10, receiver_address = $11, paid_at = $12, cancelled_at = $13, shipped_at = $14, finished_at = $15 WHERE id = $16`,
		order.OrderNo, order.UserID, order.MerchantID, string(order.Status), order.TotalAmountCent, order.PayAmountCent, order.DiscountAmountCent, order.IdempotencyKey,
		order.ReceiverName, order.ReceiverPhone, order.ReceiverAddress, order.PaidAt, order.CancelledAt, order.ShippedAt, order.FinishedAt, order.ID,
	)
	if err != nil {
		return domain.Order{}, err
	}
	return mustGetOrder(r, order.ID)
}

func (r *Repository) ListOrderEvents(orderID int64) []domain.OrderEvent {
	rows, err := r.db.Query(`SELECT id, order_id, from_status, to_status, event_type, operator_id, operator_role, remark, created_at FROM order_events WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	events := make([]domain.OrderEvent, 0)
	for rows.Next() {
		event, err := scanOrderEvent(rows)
		if err != nil {
			return events
		}
		events = append(events, event)
	}
	return events
}

func (r *Repository) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	err := r.db.QueryRow(
		`INSERT INTO order_events (order_id, from_status, to_status, event_type, operator_id, operator_role, remark, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP))
		RETURNING id, created_at`,
		event.OrderID, nullableString(event.FromStatus), event.ToStatus, event.EventType, event.OperatorID, event.OperatorRole, event.Remark, nullTime(event.CreatedAt),
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return domain.OrderEvent{}, err
	}
	return event, nil
}

func (r *Repository) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	rows, err := r.db.Query(`SELECT id, order_id, sku_id, quantity, status, locked_at, confirmed_at, released_at, created_at, updated_at FROM inventory_locks WHERE order_id = $1 ORDER BY id`, orderID)
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
	_, err := r.db.Exec(
		`UPDATE inventory_locks SET status = $1, locked_at = $2, confirmed_at = $3, released_at = $4 WHERE id = $5`,
		lock.Status, lock.LockedAt, lock.ConfirmedAt, lock.ReleasedAt, lock.ID,
	)
	return err
}

func (r *Repository) AppendBehaviorEvent(event domain.BehaviorEvent) (domain.BehaviorEvent, error) {
	err := r.db.QueryRow(
		`INSERT INTO behavior_events (user_id, event_type, note_id, product_id, sku_id, order_id, merchant_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP))
		RETURNING id, created_at`,
		nullInt64(event.UserID), event.EventType, nullInt64(event.NoteID), nullInt64(event.ProductID), nullInt64(event.SKUID), nullInt64(event.OrderID), nullInt64(event.MerchantID), nullTime(event.CreatedAt),
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return domain.BehaviorEvent{}, err
	}
	return event, nil
}

func (r *Repository) ListBehaviorEvents() []domain.BehaviorEvent {
	rows, err := r.db.Query(`SELECT id, user_id, event_type, note_id, product_id, sku_id, order_id, merchant_id, created_at FROM behavior_events ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.BehaviorEvent, 0)
	for rows.Next() {
		event, err := scanBehaviorEvent(rows)
		if err != nil {
			return out
		}
		out = append(out, event)
	}
	return out
}

func (r *Repository) CreateAITask(task domain.AIGenerationTask) (domain.AIGenerationTask, error) {
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	outputJSON, err := json.Marshal(task.Output)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	err = r.db.QueryRow(
		`INSERT INTO ai_generation_tasks (user_id, merchant_id, task_type, input_json, output_json, status, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6,$7,COALESCE($8, CURRENT_TIMESTAMP),COALESCE($9, CURRENT_TIMESTAMP))
		RETURNING id, created_at, updated_at`,
		nullInt64(task.UserID), nullInt64(task.MerchantID), task.TaskType, string(inputJSON), nullableJSON(outputJSON), task.Status, task.ErrorMessage, nullTime(task.CreatedAt), nullTime(task.UpdatedAt),
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	return task, nil
}

func (r *Repository) UpdateAITask(task domain.AIGenerationTask) error {
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return err
	}
	outputJSON, err := json.Marshal(task.Output)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE ai_generation_tasks SET user_id = $1, merchant_id = $2, task_type = $3, input_json = $4::jsonb, output_json = $5::jsonb, status = $6, error_message = $7 WHERE id = $8`,
		nullInt64(task.UserID), nullInt64(task.MerchantID), task.TaskType, string(inputJSON), nullableJSON(outputJSON), task.Status, task.ErrorMessage, task.ID,
	)
	return err
}

func (r *Repository) GetAITask(id int64) (domain.AIGenerationTask, bool) {
	row := r.db.QueryRow(`SELECT id, user_id, merchant_id, task_type, input_json, output_json, status, error_message, created_at, updated_at FROM ai_generation_tasks WHERE id = $1`, id)
	task, err := scanAITask(row)
	if err == sql.ErrNoRows {
		return domain.AIGenerationTask{}, false
	}
	return task, err == nil
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

func (r *Repository) loadNoteProductIDs(noteID int64) []int64 {
	rows, err := r.db.Query(`SELECT product_id FROM note_products WHERE note_id = $1 ORDER BY id`, noteID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var productID int64
		if err := rows.Scan(&productID); err != nil {
			return out
		}
		out = append(out, productID)
	}
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

func (r *Repository) queryUser(query string, arg any) (domain.User, error) {
	row := r.db.QueryRow(query, arg)
	var user domain.User
	err := row.Scan(&user.ID, &user.Nickname, &user.Phone, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (r *Repository) queryMerchant(query string, arg any) (domain.Merchant, error) {
	row := r.db.QueryRow(query, arg)
	var merchant domain.Merchant
	err := row.Scan(&merchant.ID, &merchant.UserID, &merchant.Name, &merchant.Description, &merchant.Status, &merchant.CreatedAt, &merchant.UpdatedAt)
	return merchant, err
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

type orderEventScanner interface {
	Scan(dest ...any) error
}

func scanOrderEvent(scanner orderEventScanner) (domain.OrderEvent, error) {
	var event domain.OrderEvent
	var fromStatus sql.NullString
	err := scanner.Scan(&event.ID, &event.OrderID, &fromStatus, &event.ToStatus, &event.EventType, &event.OperatorID, &event.OperatorRole, &event.Remark, &event.CreatedAt)
	if err != nil {
		return domain.OrderEvent{}, err
	}
	event.FromStatus = fromStatus.String
	return event, nil
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

type behaviorScanner interface {
	Scan(dest ...any) error
}

func scanBehaviorEvent(scanner behaviorScanner) (domain.BehaviorEvent, error) {
	var event domain.BehaviorEvent
	var userID, noteID, productID, skuID, orderID, merchantID sql.NullInt64
	err := scanner.Scan(&event.ID, &userID, &event.EventType, &noteID, &productID, &skuID, &orderID, &merchantID, &event.CreatedAt)
	if err != nil {
		return domain.BehaviorEvent{}, err
	}
	event.UserID = userID.Int64
	event.NoteID = noteID.Int64
	event.ProductID = productID.Int64
	event.SKUID = skuID.Int64
	event.OrderID = orderID.Int64
	event.MerchantID = merchantID.Int64
	return event, nil
}

type aiTaskScanner interface {
	Scan(dest ...any) error
}

func scanAITask(scanner aiTaskScanner) (domain.AIGenerationTask, error) {
	var task domain.AIGenerationTask
	var userID, merchantID sql.NullInt64
	var inputJSON []byte
	var outputJSON []byte
	err := scanner.Scan(&task.ID, &userID, &merchantID, &task.TaskType, &inputJSON, &outputJSON, &task.Status, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	task.UserID = userID.Int64
	task.MerchantID = merchantID.Int64
	_ = json.Unmarshal(inputJSON, &task.Input)
	if len(outputJSON) > 0 {
		_ = json.Unmarshal(outputJSON, &task.Output)
	}
	return task, nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableJSON(payload []byte) any {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	return string(payload)
}

func mustGetOrder(repo *Repository, orderID int64) (domain.Order, error) {
	order, ok := repo.GetOrder(orderID)
	if !ok {
		return domain.Order{}, fmt.Errorf("order %d not found", orderID)
	}
	return order, nil
}

func seededPasswordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", sum[:])
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
