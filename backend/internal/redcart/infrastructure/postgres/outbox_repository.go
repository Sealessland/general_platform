package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/example/redcart-copilot/backend/internal/event"
)

var _ event.OutboxStore = (*outboxStore)(nil)

type outboxStore struct {
	db dbQuerier
}

func newOutboxStore(db dbQuerier) *outboxStore {
	return &outboxStore{db: db}
}

// Append implements event.Outbox on Repository by delegating to the internal
// outbox store. This allows the application layer to pass Repository where an
// event.Outbox is required.
func (r *Repository) Append(ctx context.Context, evt event.Event) (int64, error) {
	return r.Outbox.Append(ctx, evt)
}

// Append records an event in the outbox table. It is intended to be called
// inside the same transaction that mutates business state.
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

// PollPending returns up to limit pending outbox events, oldest first.
func (s *outboxStore) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, event_type, topic, correlation_id, payload_json, occurred_at
		 FROM outbox
		 WHERE retry_count < 5
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

// MarkPublished removes the successfully published events from the outbox.
func (s *outboxStore) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query := `DELETE FROM outbox WHERE id = ANY($1::bigint[])`
	if _, err := s.db.Exec(query, ids); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

// MarkFailed increments the retry counter. When retries are exhausted, the
// record is moved to outbox_dead_letter.
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
