// Package rabbitmq provides a RabbitMQ-backed implementation of the
// event.Publisher contract used by the outbox publisher.
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/example/redcart-copilot/backend/internal/event"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends events to RabbitMQ using topic exchanges.
type Publisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	exchange string
}

// NewPublisher dials addr, declares the configured exchange and returns a
// ready-to-use Publisher. The caller is responsible for calling Close.
func NewPublisher(addr, exchange string) (*Publisher, error) {
	conn, err := amqp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if err := ch.ExchangeDeclare(
		exchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare rabbitmq exchange: %w", err)
	}
	return &Publisher{
		conn:     conn,
		channel:  ch,
		exchange: exchange,
	}, nil
}

// Publish serializes the event and sends it to the topic derived from the
// event type. The context is used for cancellation of the AMQP publish.
func (p *Publisher) Publish(ctx context.Context, evt event.Event) error {
	body, err := json.Marshal(map[string]any{
		"event_id":       evt.ID,
		"event_type":     string(evt.Type),
		"topic":          evt.Topic,
		"correlation_id": evt.CorrelationID,
		"occurred_at":    evt.OccurredAt,
		"payload":        evt.Payload,
	})
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return p.channel.PublishWithContext(
		ctx,
		p.exchange,
		evt.Topic,
		true,  // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (p *Publisher) Close() error {
	if err := p.channel.Close(); err != nil {
		_ = p.conn.Close()
		return err
	}
	return p.conn.Close()
}
