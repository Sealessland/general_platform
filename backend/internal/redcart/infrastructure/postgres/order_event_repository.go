package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

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
	return appendOrderEvent(r.db, event)
}

func appendOrderEvent(q dbQuerier, event domain.OrderEvent) (domain.OrderEvent, error) {
	err := q.QueryRow(
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
