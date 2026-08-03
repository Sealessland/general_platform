-- 事务性发件箱（outbox）：应用侧事件先与本业务数据在同一事务内落库，
-- 再由 relay 异步轮询投递；重试超限的事件迁入 outbox_dead_letter 死信表，
-- 便于人工排查与恢复。schema_migrations 由 migrate 逻辑保证幂等创建。
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(64) PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS outbox (
  id BIGSERIAL PRIMARY KEY,
  event_type VARCHAR(64) NOT NULL,
  topic VARCHAR(64) NOT NULL,
  correlation_id VARCHAR(64) NOT NULL DEFAULT '',
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL,
  retry_count INT NOT NULL DEFAULT 0,
  error_message VARCHAR(512) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox (created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_retry ON outbox (retry_count, created_at);

CREATE TABLE IF NOT EXISTS outbox_dead_letter (
  id BIGSERIAL PRIMARY KEY,
  outbox_id BIGINT NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  topic VARCHAR(64) NOT NULL,
  correlation_id VARCHAR(64) NOT NULL DEFAULT '',
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL,
  retry_count INT NOT NULL DEFAULT 0,
  error_message VARCHAR(512) NOT NULL DEFAULT '',
  failed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
