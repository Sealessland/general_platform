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

type outboxStore struct {
	db dbQuerier
}

func newOutboxStore(db dbQuerier) *outboxStore {
	return &outboxStore{db: db}
}

func (r *Repository) Append(ctx context.Context, evt event.Event) (int64, error) {
	return r.Outbox.Append(ctx, evt)
}

func (r *Repository) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	return r.Outbox.PollPending(ctx, limit)
}

func (r *Repository) MarkPublished(ctx context.Context, ids []int64) error {
	return r.Outbox.MarkPublished(ctx, ids)
}

func (r *Repository) MarkFailed(ctx context.Context, id int64, reason string) error {
	return r.Outbox.MarkFailed(ctx, id, reason)
}

func (r *Repository) BeginTx(ctx context.Context) (event.OutboxTx, error) {
	return r.Outbox.BeginTx(ctx)
}

func (r *Repository) PollPendingInTx(ctx context.Context, tx event.OutboxTx, limit int) ([]event.Event, error) {
	return r.Outbox.PollPendingInTx(ctx, tx, limit)
}

func (r *Repository) MarkPublishedInTx(ctx context.Context, tx event.OutboxTx, ids []int64) error {
	return r.Outbox.MarkPublishedInTx(ctx, tx, ids)
}

func (r *Repository) MarkFailedInTx(ctx context.Context, tx event.OutboxTx, id int64, reason string) error {
	return r.Outbox.MarkFailedInTx(ctx, tx, id, reason)
}

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

type outboxTx struct {
	tx *gormTx
}

func (t *outboxTx) Commit() error   { return t.tx.Commit() }
func (t *outboxTx) Rollback() error { return t.tx.Rollback() }

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
