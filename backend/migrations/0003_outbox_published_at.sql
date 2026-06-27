-- 0003_outbox_published_at.sql
-- Add published_at column to outbox table for soft-delete semantics.
-- Published events are retained (marked with published_at) instead of deleted,
-- enabling audit trails and crash-safe relay processing.

ALTER TABLE outbox ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

-- Partial index for pending events only (published_at IS NULL).
-- Used by the relay's PollPending / PollPendingInTx queries.
CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON outbox (created_at)
    WHERE published_at IS NULL;

-- Index for dead-letter recovery by failure time.
CREATE INDEX IF NOT EXISTS idx_outbox_dead_letter_failed_at
    ON outbox_dead_letter (occurred_at);
