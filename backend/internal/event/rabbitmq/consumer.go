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
	defaultPrefetch = 10
	dlxExchange     = "redcart.events.dlx"
	dlqName         = "redcart.events.dlq"
)

// Handler processes a consumed event. Returning an error triggers nack with
// requeue=false, sending the message to the dead-letter exchange. Business
// logic lives here; the Consumer owns delivery/ack/dedup mechanics.
type Handler interface {
	Handle(ctx context.Context, evt event.Event) error
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, evt event.Event) error

func (f HandlerFunc) Handle(ctx context.Context, evt event.Event) error { return f(ctx, evt) }

// Deduplicator provides idempotency for consumers. RabbitMQ may redeliver
// messages after a consumer crash or network blip, so handlers must guard
// against duplicate side-effects using the outbox event ID.
type Deduplicator interface {
	// IsDuplicate returns true if the event was already processed.
	IsDuplicate(ctx context.Context, eventID int64) (bool, error)
	// MarkProcessed records that the event has been processed.
	MarkProcessed(ctx context.Context, eventID int64) error
}

// MemoryDeduplicator is an in-process deduplicator for demos. Production
// should use a Redis-backed or DB-backed implementation that survives
// consumer restarts.
type MemoryDeduplicator struct {
	mu   sync.Mutex
	seen map[int64]bool
}

func NewMemoryDeduplicator() *MemoryDeduplicator {
	return &MemoryDeduplicator{seen: make(map[int64]bool)}
}

func (m *MemoryDeduplicator) IsDuplicate(_ context.Context, eventID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen[eventID], nil
}

func (m *MemoryDeduplicator) MarkProcessed(_ context.Context, eventID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[eventID] = true
	return nil
}

// Acknowledger abstracts ack/nack so the consume logic is testable without
// a live AMQP channel. amqp.Delivery satisfies this interface.
type Acknowledger interface {
	Ack(multiple bool) error
	Nack(multiple, requeue bool) error
}

// Consumer subscribes to a RabbitMQ queue and dispatches messages to a Handler
// with manual ack, prefetch-based QoS, idempotency dedup, and DLX on failure.
type Consumer struct {
	addr      string
	exchange  string
	queueName string
	prefetch  int
	handler   Handler
	dedup     Deduplicator
	logger    *log.Logger

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

type ConsumerConfig struct {
	QueueName string
	Prefetch  int
	Logger    *log.Logger
}

func NewConsumer(addr, exchange string, handler Handler, dedup Deduplicator, cfg ConsumerConfig) (*Consumer, error) {
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = defaultPrefetch
	}
	if cfg.QueueName == "" {
		cfg.QueueName = "redcart.events.consumer"
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	c := &Consumer{
		addr:      addr,
		exchange:  exchange,
		queueName: cfg.QueueName,
		prefetch:  cfg.Prefetch,
		handler:   handler,
		dedup:     dedup,
		logger:    cfg.Logger,
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Consumer) connect() error {
	conn, err := amqp.Dial(c.addr)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}

	// Dead-letter exchange (fanout) + dead-letter queue.
	if err := ch.ExchangeDeclare(dlxExchange, "fanout", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare DLX exchange: %w", err)
	}
	dlq, err := ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare DLQ: %w", err)
	}
	if err := ch.QueueBind(dlq.Name, "#", dlxExchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("bind DLQ: %w", err)
	}

	// Main queue with x-dead-letter-exchange argument — nacked messages
	// (requeue=false) are routed to the DLX instead of being dropped.
	args := amqp.Table{"x-dead-letter-exchange": dlxExchange}
	q, err := ch.QueueDeclare(c.queueName, true, false, false, false, args)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare queue: %w", err)
	}
	if err := ch.QueueBind(q.Name, "#", c.exchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("bind queue: %w", err)
	}

	// QoS prefetch — limit unacked messages per consumer to prevent
	// overwhelming a slow handler with a backlog of in-flight deliveries.
	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("set qos: %w", err)
	}

	c.conn = conn
	c.channel = ch
	return nil
}

// Start begins consuming. It blocks until Close is called or the channel
// is lost.
func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		c.queueName,
		"",    // consumer tag
		false, // autoAck — manual ack for reliability
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	for d := range msgs {
		c.processDelivery(ctx, d.Body, d)
	}
	return nil
}

// processDelivery is the core consume loop. It is extracted so tests can
// drive it with a mock Acknowledger and body without a live AMQP channel.
// amqp.Delivery satisfies Acknowledger via its Ack/Nack methods.
func (c *Consumer) processDelivery(ctx context.Context, body []byte, acker Acknowledger) {
	evt, err := decodeBody(body)
	if err != nil {
		c.logger.Printf("consumer: decode failed: %v — sending to DLX", err)
		_ = acker.Nack(false, false) // requeue=false → DLX
		return
	}

	// Idempotency — skip redelivered messages that were already processed.
	if dup, err := c.dedup.IsDuplicate(ctx, evt.ID); err != nil {
		c.logger.Printf("consumer: dedup check failed for event %d: %v — requeue", evt.ID, err)
		_ = acker.Nack(false, true) // requeue=true, transient error
		return
	} else if dup {
		_ = acker.Ack(false)
		return
	}

	// Business logic.
	if err := c.handler.Handle(ctx, evt); err != nil {
		c.logger.Printf("consumer: handler failed for event %d: %v — sending to DLX", evt.ID, err)
		_ = acker.Nack(false, false) // requeue=false → DLX
		return
	}

	// Mark processed BEFORE ack so a crash between MarkProcessed and Ack
	// just causes a redelivery which IsDuplicate will skip. The reverse
	// order (Ack then Mark) would risk executing side-effects twice.
	if err := c.dedup.MarkProcessed(ctx, evt.ID); err != nil {
		c.logger.Printf("consumer: mark processed failed for event %d: %v", evt.ID, err)
	}
	_ = acker.Ack(false)
}

func decodeBody(body []byte) (event.Event, error) {
	var msg struct {
		EventID       int64           `json:"event_id"`
		EventType     string          `json:"event_type"`
		Topic         string          `json:"topic"`
		CorrelationID string          `json:"correlation_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return event.Event{}, fmt.Errorf("unmarshal message: %w", err)
	}
	return event.Event{
		ID:            msg.EventID,
		Type:          event.Type(msg.EventType),
		Topic:         msg.Topic,
		CorrelationID: msg.CorrelationID,
		Payload:       msg.Payload,
		OccurredAt:    msg.OccurredAt,
	}, nil
}

func (c *Consumer) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}