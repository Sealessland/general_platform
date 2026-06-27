// Package rabbitmq provides a RabbitMQ-backed implementation of the
// event.Publisher contract used by the outbox publisher.
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	reconnectDelay = 3 * time.Second
	publishTimeout = 10 * time.Second
)

type Publisher struct {
	addr     string
	exchange string
	logger   *log.Logger

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

func NewPublisher(addr, exchange string) (*Publisher, error) {
	p := &Publisher{
		addr:     addr,
		exchange: exchange,
		logger:   log.Default(),
	}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Publisher) connect() error {
	conn, err := amqp.Dial(p.addr)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if err := ch.ExchangeDeclare(
		p.exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare rabbitmq exchange: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	p.conn = conn
	p.channel = ch

	go p.watchClose()
	return nil
}

func (p *Publisher) watchClose() {
	closeCh := p.channel.NotifyClose(make(chan *amqp.Error, 1))
	err, ok := <-closeCh
	if !ok {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.logger.Printf("rabbitmq channel closed: %v; attempting reconnect", err)
	p.mu.Unlock()

	for {
		time.Sleep(reconnectDelay)
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if err := p.connect(); err != nil {
			p.logger.Printf("rabbitmq reconnect failed: %v", err)
			p.mu.Unlock()
			continue
		}
		p.logger.Printf("rabbitmq reconnected successfully")
		p.mu.Unlock()
		return
	}
}

func (p *Publisher) ensureConnected() error {
	if p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	return fmt.Errorf("rabbitmq channel unavailable")
}

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

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureConnected(); err != nil {
		return err
	}

	confSeq, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		p.exchange,
		evt.Topic,
		true,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish to rabbitmq: %w", err)
	}

	confirmCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	confirmed, err := confSeq.WaitContext(confirmCtx)
	if err != nil {
		return fmt.Errorf("wait publisher confirm: %w", err)
	}
	if !confirmed {
		return fmt.Errorf("rabbitmq broker nack for event %d (topic %s)", evt.ID, evt.Topic)
	}
	return nil
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
