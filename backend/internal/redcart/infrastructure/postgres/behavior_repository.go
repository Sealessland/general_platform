package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

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
