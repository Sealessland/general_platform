CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  nickname VARCHAR(100) NOT NULL,
  phone VARCHAR(32) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_users_phone ON users (phone);

CREATE TABLE merchants (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  name VARCHAR(120) NOT NULL,
  description TEXT,
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_merchants_user_id ON merchants (user_id);
CREATE INDEX idx_merchants_status ON merchants (status);

CREATE TABLE notes (
  id BIGSERIAL PRIMARY KEY,
  author_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL,
  content TEXT NOT NULL,
  cover_url VARCHAR(512),
  status VARCHAR(32) NOT NULL,
  view_count BIGINT NOT NULL DEFAULT 0,
  like_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notes_author_status ON notes (author_id, status);

CREATE TABLE note_products (
  id BIGSERIAL PRIMARY KEY,
  note_id BIGINT NOT NULL,
  product_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_note_products ON note_products (note_id, product_id);
CREATE INDEX idx_note_products_product_id ON note_products (product_id);

CREATE TABLE products (
  id BIGSERIAL PRIMARY KEY,
  merchant_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT,
  cover_url VARCHAR(512),
  category_id BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  selling_points JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_products_merchant_status ON products (merchant_id, status);

CREATE TABLE product_skus (
  id BIGSERIAL PRIMARY KEY,
  product_id BIGINT NOT NULL,
  sku_name VARCHAR(128) NOT NULL,
  sku_attrs_json JSONB,
  price_cent BIGINT NOT NULL,
  stock INT NOT NULL,
  locked_stock INT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_product_skus_product_id ON product_skus (product_id);

CREATE TABLE carts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_carts_user_id ON carts (user_id);

CREATE TABLE cart_items (
  id BIGSERIAL PRIMARY KEY,
  cart_id BIGINT,
  user_id BIGINT NOT NULL,
  product_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  quantity INT NOT NULL,
  selected BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_cart_items_user_id ON cart_items (user_id);
CREATE INDEX idx_cart_items_product_sku ON cart_items (product_id, sku_id);

CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  order_no VARCHAR(64) NOT NULL,
  user_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL,
  total_amount_cent BIGINT NOT NULL,
  pay_amount_cent BIGINT NOT NULL,
  discount_amount_cent BIGINT NOT NULL DEFAULT 0,
  idempotency_key VARCHAR(128) NOT NULL,
  receiver_name VARCHAR(120) NOT NULL,
  receiver_phone VARCHAR(32) NOT NULL,
  receiver_address VARCHAR(512) NOT NULL,
  paid_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  shipped_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_orders_order_no ON orders (order_no);
CREATE UNIQUE INDEX uk_user_idempotency_key ON orders (user_id, idempotency_key);
CREATE INDEX idx_orders_user_status ON orders (user_id, status);
CREATE INDEX idx_orders_merchant_status ON orders (merchant_id, status);

CREATE TABLE order_items (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL,
  product_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  product_title_snapshot VARCHAR(255) NOT NULL,
  sku_name_snapshot VARCHAR(128) NOT NULL,
  price_cent_snapshot BIGINT NOT NULL,
  quantity INT NOT NULL,
  total_amount_cent BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_order_items_order_id ON order_items (order_id);

CREATE TABLE order_events (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL,
  from_status VARCHAR(32),
  to_status VARCHAR(32) NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  operator_id BIGINT NOT NULL DEFAULT 0,
  operator_role VARCHAR(32) NOT NULL DEFAULT '',
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_order_events_order_id ON order_events (order_id);

CREATE TABLE inventory_locks (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  quantity INT NOT NULL,
  status VARCHAR(32) NOT NULL,
  locked_at TIMESTAMPTZ NOT NULL,
  confirmed_at TIMESTAMPTZ,
  released_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uk_inventory_locks_order_sku ON inventory_locks (order_id, sku_id);
CREATE INDEX idx_inventory_locks_sku_status ON inventory_locks (sku_id, status);

CREATE TABLE behavior_events (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT,
  event_type VARCHAR(64) NOT NULL,
  note_id BIGINT,
  product_id BIGINT,
  sku_id BIGINT,
  order_id BIGINT,
  merchant_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_behavior_events_merchant_time ON behavior_events (merchant_id, created_at);
CREATE INDEX idx_behavior_events_product_time ON behavior_events (product_id, created_at);

CREATE TABLE ai_generation_tasks (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT,
  merchant_id BIGINT,
  task_type VARCHAR(64) NOT NULL,
  input_json JSONB NOT NULL,
  output_json JSONB,
  status VARCHAR(32) NOT NULL,
  error_message VARCHAR(512) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ai_generation_tasks_merchant ON ai_generation_tasks (merchant_id, task_type, status);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_merchants_updated_at
BEFORE UPDATE ON merchants
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_notes_updated_at
BEFORE UPDATE ON notes
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_products_updated_at
BEFORE UPDATE ON products
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_product_skus_updated_at
BEFORE UPDATE ON product_skus
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_carts_updated_at
BEFORE UPDATE ON carts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_cart_items_updated_at
BEFORE UPDATE ON cart_items
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_orders_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_order_items_updated_at
BEFORE UPDATE ON order_items
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_inventory_locks_updated_at
BEFORE UPDATE ON inventory_locks
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_ai_generation_tasks_updated_at
BEFORE UPDATE ON ai_generation_tasks
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
