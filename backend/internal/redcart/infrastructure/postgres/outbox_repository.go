package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/example/redcart-copilot/backend/internal/event"
)

var (
	_ event.OutboxStore      = (*outboxStore)(nil)
	_ event.OutboxRelayStore = (*outboxStore)(nil)
	_ event.OutboxRelayStore = (*Repository)(nil)
)

// outboxStore 实现 event.OutboxStore 与 event.OutboxRelayStore，
// 负责出站事件的追加、待发布事件的轮询，以及失败重试与死信迁移。
type outboxStore struct {
	db dbQuerier
}

// newOutboxStore 以给定查询器构造 outbox 存储。
func newOutboxStore(db dbQuerier) *outboxStore {
	return &outboxStore{db: db}
}

// Append 追加一条出站事件（委托给 Outbox 存储）。
func (r *Repository) Append(ctx context.Context, evt event.Event) (int64, error) {
	return r.Outbox.Append(ctx, evt)
}

// PollPending 轮询待发布事件（委托给 Outbox 存储）。
func (r *Repository) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	return r.Outbox.PollPending(ctx, limit)
}

// MarkPublished 将指定事件标记为已发布（委托给 Outbox 存储）。
func (r *Repository) MarkPublished(ctx context.Context, ids []int64) error {
	return r.Outbox.MarkPublished(ctx, ids)
}

// MarkFailed 标记事件发送失败并累计重试（委托给 Outbox 存储）。
func (r *Repository) MarkFailed(ctx context.Context, id int64, reason string) error {
	return r.Outbox.MarkFailed(ctx, id, reason)
}

// BeginTx 开启 outbox 事务（委托给 Outbox 存储）。
func (r *Repository) BeginTx(ctx context.Context) (event.OutboxTx, error) {
	return r.Outbox.BeginTx(ctx)
}

// PollPendingInTx 在事务内轮询待发布事件（委托给 Outbox 存储）。
func (r *Repository) PollPendingInTx(ctx context.Context, tx event.OutboxTx, limit int) ([]event.Event, error) {
	return r.Outbox.PollPendingInTx(ctx, tx, limit)
}

// MarkPublishedInTx 在事务内标记已发布（委托给 Outbox 存储）。
func (r *Repository) MarkPublishedInTx(ctx context.Context, tx event.OutboxTx, ids []int64) error {
	return r.Outbox.MarkPublishedInTx(ctx, tx, ids)
}

// MarkFailedInTx 在事务内标记失败（委托给 Outbox 存储）。
func (r *Repository) MarkFailedInTx(ctx context.Context, tx event.OutboxTx, id int64, reason string) error {
	return r.Outbox.MarkFailedInTx(ctx, tx, id, reason)
}

// Append 向 outbox 表插入一条待发布事件，payload 为空时回退为 {}。
func (s *outboxStore) Append(ctx context.Context, evt event.Event) (int64, error) {
	payload := evt.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO outbox (event_type, topic, correlation_id, payload_json, occurred_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		string(evt.Type), evt.Topic, evt.CorrelationID, string(payload), evt.OccurredAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("append outbox event: %w", err)
	}
	return id, nil
}

// PollPending 轮询未发布且重试未达上限的事件，按创建时间升序。
func (s *outboxStore) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, event_type, topic, correlation_id, payload_json, occurred_at
		 FROM outbox
		 WHERE published_at IS NULL AND retry_count < 5
		 ORDER BY created_at ASC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("poll outbox: %w", err)
	}
	defer rows.Close()
	return scanOutboxRows(rows)
}

// MarkPublished 批量将事件标记为已发布。
func (s *outboxStore) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE outbox SET published_at = NOW() WHERE id = ANY($1::bigint[])`,
		ids,
	)
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

// MarkFailed 累加该事件的失败重试次数并记录错误信息；一旦达到重试上限，
// 事件会被迁出到 outbox_dead_letter 死信表，避免无限重试并保留排查线索。
func (s *outboxStore) MarkFailed(ctx context.Context, id int64, reason string) error {
	const maxRetries = 5
	result, err := s.db.Exec(
		`UPDATE outbox
		 SET retry_count = retry_count + 1, error_message = $1
		 WHERE id = $2 AND retry_count < $3`,
		reason, id, maxRetries,
	)
	if err != nil {
		return fmt.Errorf("mark outbox failed: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		_, err = s.db.Exec(
			`WITH moved AS (
				DELETE FROM outbox WHERE id = $1
				RETURNING id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message
			)
			INSERT INTO outbox_dead_letter (outbox_id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message)
			SELECT id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message
			FROM moved`,
			id,
		)
		if err != nil {
			return fmt.Errorf("move outbox to dead letter: %w", err)
		}
	}
	return nil
}

// BeginTx 开启底层数据库事务并包装为 event.OutboxTx。
func (s *outboxStore) BeginTx(ctx context.Context) (event.OutboxTx, error) {
	sqlDB, ok := s.db.(*gormSQL)
	if !ok {
		return nil, fmt.Errorf("outbox BeginTx: underlying db must be *gormSQL")
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin outbox tx: %w", err)
	}
	return &outboxTx{tx: tx}, nil
}

// outboxTx 是事务版 outbox 存储的适配器，把 *gormTx 包装为 event.OutboxTx，
// 使事件投递能与业务数据变更在同一事务中提交。
type outboxTx struct {
	tx *gormTx
}

// Commit 提交 outbox 事务。
func (t *outboxTx) Commit() error { return t.tx.Commit() }

// Rollback 回滚 outbox 事务。
func (t *outboxTx) Rollback() error { return t.tx.Rollback() }

// PollPendingInTx 在事务内以 FOR UPDATE SKIP LOCKED 轮询待发布事件：
// 多个 relay 实例并发消费时互不阻塞、互不重复领取同一事件。
func (s *outboxStore) PollPendingInTx(ctx context.Context, tx event.OutboxTx, limit int) ([]event.Event, error) {
	otx, ok := tx.(*outboxTx)
	if !ok {
		return nil, fmt.Errorf("PollPendingInTx: tx must be *outboxTx")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := otx.tx.Query(
		`SELECT id, event_type, topic, correlation_id, payload_json, occurred_at
		 FROM outbox
		 WHERE published_at IS NULL AND retry_count < 5
		 ORDER BY created_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("poll outbox in tx: %w", err)
	}
	defer rows.Close()
	return scanOutboxRows(rows)
}

// MarkPublishedInTx 在事务内批量标记事件为已发布。
func (s *outboxStore) MarkPublishedInTx(ctx context.Context, tx event.OutboxTx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	otx, ok := tx.(*outboxTx)
	if !ok {
		return fmt.Errorf("MarkPublishedInTx: tx must be *outboxTx")
	}
	_, err := otx.tx.Exec(
		`UPDATE outbox SET published_at = NOW() WHERE id = ANY($1::bigint[])`,
		ids,
	)
	if err != nil {
		return fmt.Errorf("mark outbox published in tx: %w", err)
	}
	return nil
}

// MarkFailedInTx 在事务内累加重试次数，超过上限迁入死信表。
func (s *outboxStore) MarkFailedInTx(ctx context.Context, tx event.OutboxTx, id int64, reason string) error {
	otx, ok := tx.(*outboxTx)
	if !ok {
		return fmt.Errorf("MarkFailedInTx: tx must be *outboxTx")
	}
	const maxRetries = 5
	result, err := otx.tx.Exec(
		`UPDATE outbox
		 SET retry_count = retry_count + 1, error_message = $1
		 WHERE id = $2 AND retry_count < $3`,
		reason, id, maxRetries,
	)
	if err != nil {
		return fmt.Errorf("mark outbox failed in tx: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		_, err = otx.tx.Exec(
			`WITH moved AS (
				DELETE FROM outbox WHERE id = $1
				RETURNING id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message
			)
			INSERT INTO outbox_dead_letter (outbox_id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message)
			SELECT id, event_type, topic, correlation_id, payload_json, occurred_at, retry_count, error_message
			FROM moved`,
			id,
		)
		if err != nil {
			return fmt.Errorf("move outbox to dead letter in tx: %w", err)
		}
	}
	return nil
}

// appendOutboxEvent 在事务内追加 outbox 事件（供订单事务副作用复用），
// 与 outboxStore.Append 逻辑一致，但作用于事务连接。
func appendOutboxEvent(q dbQuerier, evt event.Event) (int64, error) {
	payload := evt.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	var id int64
	err := q.QueryRow(
		`INSERT INTO outbox (event_type, topic, correlation_id, payload_json, occurred_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		string(evt.Type), evt.Topic, evt.CorrelationID, string(payload), evt.OccurredAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("append outbox event: %w", err)
	}
	return id, nil
}

// scanOutboxRows 将查询结果行解码为 event.Event 列表，payload 为 NULL 时置空。
func scanOutboxRows(rows *sql.Rows) ([]event.Event, error) {
	var events []event.Event
	for rows.Next() {
		var evt event.Event
		var payload []byte
		err := rows.Scan(&evt.ID, &evt.Type, &evt.Topic, &evt.CorrelationID, &payload, &evt.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		if len(payload) > 0 {
			evt.Payload = json.RawMessage(payload)
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}
