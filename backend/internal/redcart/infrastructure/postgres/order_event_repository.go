package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// ListOrderEvents 返回订单的全部状态事件，按 ID 升序。
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

// AppendOrderEvent 追加一条订单事件并返回带 ID 与创建时间的事件。
func (r *Repository) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	return appendOrderEvent(r.db, event)
}

// appendOrderEvent 是事务版追加订单事件的辅助函数（供 pgOrderTx 复用），
// 与仓储直接调用走同一实现。
func appendOrderEvent(q dbQuerier, event domain.OrderEvent) (domain.OrderEvent, error) {
	err := q.QueryRow(
		`INSERT INTO order_events (order_id, from_status, to_status, event_type, operator_id, operator_role, remark, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8, CURRENT_TIMESTAMP))
		RETURNING id, created_at`,
		event.OrderID, nullableString(event.FromStatus), event.ToStatus, event.EventType, event.OperatorID, event.OperatorRole, event.Remark, timeToSQL(event.CreatedAt),
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return domain.OrderEvent{}, err
	}
	return event, nil
}

type orderEventScanner interface {
	Scan(dest ...any) error
}

// scanOrderEvent 解码订单事件；from_status 可空（订单的首条事件）。
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
