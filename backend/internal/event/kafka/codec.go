package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// message is the stable JSON shape stored in Kafka. Keeping this transport
// type here prevents Kafka details from leaking into the application layer.
type message struct {
	EventID       int64           `json:"event_id"`
	EventType     event.Type      `json:"event_type"`
	EventTopic    string          `json:"event_topic"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	OccurredAt    time.Time       `json:"occurred_at"`
}

func encodeEvent(evt event.Event) ([]byte, error) {
	value, err := json.Marshal(message{
		EventID:       evt.ID,
		EventType:     evt.Type,
		EventTopic:    evt.Topic,
		CorrelationID: evt.CorrelationID,
		Payload:       evt.Payload,
		OccurredAt:    evt.OccurredAt,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Kafka event: %w", err)
	}
	return value, nil
}

func decodeEvent(value []byte) (event.Event, error) {
	var decoded message
	if err := json.Unmarshal(value, &decoded); err != nil {
		return event.Event{}, fmt.Errorf("decode Kafka event: %w", err)
	}
	return event.Event{
		ID:            decoded.EventID,
		Type:          decoded.EventType,
		Topic:         decoded.EventTopic,
		CorrelationID: decoded.CorrelationID,
		Payload:       decoded.Payload,
		OccurredAt:    decoded.OccurredAt,
	}, nil
}
