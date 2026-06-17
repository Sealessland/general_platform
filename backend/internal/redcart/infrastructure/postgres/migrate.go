package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func (r *Repository) migrate(ctx context.Context) error {
	if err := r.ensureSchemaMigrationsTable(ctx); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	dir, err := resolveMigrationsDir()
	if err != nil {
		return err
	}
	files, err := listMigrationFiles(dir)
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	for _, file := range files {
		version := filepath.Base(file)
		applied, err := r.isMigrationApplied(ctx, version)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}
		sqlText, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if err := r.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(sqlText)).Error; err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?) ON CONFLICT DO NOTHING`, version).Error
		}); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
	}
	return nil
}

func (r *Repository) ensureSchemaMigrationsTable(ctx context.Context) error {
	return r.gormDB.WithContext(ctx).Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(64) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`).Error
}

func (r *Repository) isMigrationApplied(ctx context.Context, version string) (bool, error) {
	var count int64
	err := r.gormDB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Row().Scan(&count)
	return count > 0, err
}

func resolveMigrationsDir() (string, error) {
	candidates := []string{
		envOrDefault("MIGRATIONS_DIR", ""),
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if ok {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "migrations"),
			filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "..", "migrations"),
		)
	}

	candidates = append(candidates,
		"migrations",
		"/app/migrations",
	)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found")
}

func listMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

func (r *Repository) seed(ctx context.Context) error {
	var count int
	if err := r.gormDB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM users`).Row().Scan(&count); err != nil {
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
  (2, 1, 'Travel Makeup Organizer', 'Portable makeup storage for dorm, travel, and desk organization.', 'https://images.example.com/makeup-organizer.jpg', 102, 'online', '["Compartment layout","Portable and lightweight","Easy to clean"]'::jsonb),
  (3, 1, 'LED Desk Lamp', 'Dimmable LED desk lamp with warm/cold light modes for small desk setups.', 'https://images.example.com/desk-lamp.jpg', 103, 'online', '["3 color temperatures","USB powered","Space saving"]'::jsonb),
  (4, 1, 'Desk Storage Box Set', 'Multi-size storage boxes for stationery, cables, and daily desk items.', 'https://images.example.com/storage-box.jpg', 104, 'online', '["Stackable","See-through lid","Cable hole design"]'::jsonb),
  (5, 1, 'USB Power Strip', 'Compact power strip with USB-C and USB-A ports for dorm desk electronics.', 'https://images.example.com/power-strip.jpg', 105, 'online', '["3 AC + 3 USB","Overload protection","1.8m cord"]'::jsonb)
ON CONFLICT DO NOTHING;

INSERT INTO product_skus (id, product_id, sku_name, sku_attrs_json, price_cent, stock, locked_stock, status) VALUES
  (1, 1, 'Cherry Set', '{"shade":"cherry","pack":"3pcs"}'::jsonb, 12900, 29, 1, 'active'),
  (2, 1, 'Rose Set', '{"shade":"rose","pack":"3pcs"}'::jsonb, 13900, 18, 1, 'active'),
  (3, 2, 'Cream White', '{"color":"cream","size":"standard"}'::jsonb, 8900, 38, 0, 'active'),
  (4, 3, 'White', '{"color":"white","size":"standard"}'::jsonb, 7900, 50, 0, 'active'),
  (5, 4, '3-Piece Set', '{"color":"clear","size":"3pcs"}'::jsonb, 4900, 60, 0, 'active'),
  (6, 5, 'Standard', '{"color":"white","size":"standard"}'::jsonb, 5900, 45, 0, 'active')
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

	if err := r.gormDB.WithContext(ctx).Exec(seedSQL).Error; err != nil {
		return fmt.Errorf("seed postgres: %w", err)
	}
	return nil
}

func seededPasswordHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("seed password hash: %v", err))
	}
	return string(hash)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
