package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	kafkago "github.com/segmentio/kafka-go"
)

const (
	defaultGroupID  = "redcart.events.consumer"
	defaultDLQTopic = "redcart.events.dlq"
	defaultMinBytes = 1
	defaultMaxBytes = 10e6
	defaultMaxWait  = 500 * time.Millisecond
)

// Handler processes a consumed event. Returning an error writes the original
// Kafka record to the DLQ topic and commits the source offset.
type Handler interface {
	Handle(ctx context.Context, evt event.Event) error
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, evt event.Event) error

// Handle calls the wrapped function.
func (f HandlerFunc) Handle(ctx context.Context, evt event.Event) error { return f(ctx, evt) }

// Deduplicator provides idempotency for at-least-once Kafka consumers.
type Deduplicator interface {
	// IsDuplicate returns true if the event was already processed.
	IsDuplicate(ctx context.Context, eventID int64) (bool, error)
	// MarkProcessed records that the event has been processed.
	MarkProcessed(ctx context.Context, eventID int64) error
}

// MemoryDeduplicator is an in-process deduplicator for demos. Production should
// use Redis SETNX or a database-backed table so processed offsets survive restarts.
type MemoryDeduplicator struct {
	mu   sync.Mutex
	seen map[int64]bool
}

// NewMemoryDeduplicator creates an in-memory deduplicator.
func NewMemoryDeduplicator() *MemoryDeduplicator {
	return &MemoryDeduplicator{seen: make(map[int64]bool)}
}

// IsDuplicate reports whether eventID has already been processed.
func (m *MemoryDeduplicator) IsDuplicate(_ context.Context, eventID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen[eventID], nil
}

// MarkProcessed records eventID as processed.
func (m *MemoryDeduplicator) MarkProcessed(_ context.Context, eventID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[eventID] = true
	return nil
}

type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

// Consumer subscribes to Kafka event topics with explicit offset commits.
type Consumer struct {
	reader    *kafkago.Reader
	dlqWriter messageWriter
	dlqTopic  string
	handler   Handler
	dedup     Deduplicator
	logger    *log.Logger
}

// ConsumerConfig configures a Kafka consumer. Topics are logical event topics;
// TopicPrefix is applied to each topic unless it is already present.
type ConsumerConfig struct {
	GroupID     string
	Topics      []string
	TopicPrefix string
	DLQTopic    string
	MinBytes    int
	MaxBytes    int
	MaxWait     time.Duration
	Logger      *log.Logger
}

// NewConsumer creates a Kafka consumer using consumer-group explicit commits.
func NewConsumer(brokers []string, handler Handler, dedup Deduplicator, cfg ConsumerConfig) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("handler is required")
	}
	if dedup == nil {
		return nil, fmt.Errorf("deduplicator is required")
	}
	if cfg.GroupID == "" {
		cfg.GroupID = defaultGroupID
	}
	if cfg.MinBytes <= 0 {
		cfg.MinBytes = defaultMinBytes
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultMaxBytes
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = defaultMaxWait
	}
	if cfg.DLQTopic == "" {
		cfg.DLQTopic = defaultDLQTopic
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	topics := qualifyTopics(cfg.TopicPrefix, cfg.Topics)
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        cfg.GroupID,
		GroupTopics:    topics,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		MaxWait:        cfg.MaxWait,
		CommitInterval: 0,
		ErrorLogger:    cfg.Logger,
	})
	dlqWriter := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  qualifyTopic(cfg.TopicPrefix, cfg.DLQTopic),
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		Async:                  false,
		BatchTimeout:           10 * time.Millisecond,
		ReadTimeout:            publishTimeout,
		WriteTimeout:           publishTimeout,
		AllowAutoTopicCreation: true,
		ErrorLogger:            cfg.Logger,
	}
	return &Consumer{
		reader:    reader,
		dlqWriter: dlqWriter,
		dlqTopic:  qualifyTopic(cfg.TopicPrefix, cfg.DLQTopic),
		handler:   handler,
		dedup:     dedup,
		logger:    cfg.Logger,
	}, nil
}

// Start consumes messages until the context is cancelled or a non-committable
// processing error occurs.
func (c *Consumer) Start(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("fetch kafka message: %w", err)
		}
		commit, err := c.processMessage(ctx, msg)
		if err != nil && !commit {
			return err
		}
		if commit {
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit kafka offset topic=%s partition=%d offset=%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafkago.Message) (bool, error) {
	evt, err := decodeBody(msg.Value)
	if err != nil {
		c.logger.Printf("kafka consumer: decode failed: %v — writing to DLQ", err)
		return c.deadLetter(ctx, msg, "decode_failed", err)
	}

	dup, err := c.dedup.IsDuplicate(ctx, evt.ID)
	if err != nil {
		return false, fmt.Errorf("kafka consumer: dedup check failed for event %d: %w", evt.ID, err)
	}
	if dup {
		return true, nil
	}

	if err := c.handler.Handle(ctx, evt); err != nil {
		c.logger.Printf("kafka consumer: handler failed for event %d: %v — writing to DLQ", evt.ID, err)
		return c.deadLetter(ctx, msg, "handler_failed", err)
	}

	if err := c.dedup.MarkProcessed(ctx, evt.ID); err != nil {
		return false, fmt.Errorf("kafka consumer: mark processed failed for event %d: %w", evt.ID, err)
	}
	return true, nil
}

func (c *Consumer) deadLetter(ctx context.Context, msg kafkago.Message, reason string, cause error) (bool, error) {
	dlqCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	dlq := kafkago.Message{
		Topic: c.dlqTopic,
		Key:   msg.Key,
		Value: msg.Value,
		Time:  time.Now().UTC(),
		Headers: append(cloneHeaders(msg.Headers),
			kafkago.Header{Key: "dlq_reason", Value: []byte(reason)},
			kafkago.Header{Key: "dlq_error", Value: []byte(cause.Error())},
			kafkago.Header{Key: "source_topic", Value: []byte(msg.Topic)},
			kafkago.Header{Key: "source_partition", Value: []byte(strconv.Itoa(msg.Partition))},
			kafkago.Header{Key: "source_offset", Value: []byte(strconv.FormatInt(msg.Offset, 10))},
		),
	}
	if err := c.dlqWriter.WriteMessages(dlqCtx, dlq); err != nil {
		return false, fmt.Errorf("write kafka DLQ topic %s: %w", c.dlqTopic, err)
	}
	return true, nil
}

// decodeBody deserializes the shared event envelope emitted by Publisher.Publish.
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

// Close releases the reader and DLQ writer.
func (c *Consumer) Close() error {
	var err error
	if c.reader != nil {
		err = errors.Join(err, c.reader.Close())
	}
	if c.dlqWriter != nil {
		err = errors.Join(err, c.dlqWriter.Close())
	}
	return err
}

func qualifyTopics(prefix string, topics []string) []string {
	if len(topics) == 0 {
		topics = []string{
			event.TypeOrderCreated.Topic(),
			event.TypeOrderPaid.Topic(),
			event.TypeOrderCancelled.Topic(),
			event.TypeOrderShipped.Topic(),
			event.TypeOrderFinished.Topic(),
			event.TypeOrderRefundRequested.Topic(),
			event.TypeOrderRefunded.Topic(),
			event.TypeBehaviorNoteView.Topic(),
			event.TypeBehaviorProductClick.Topic(),
			event.TypeBehaviorAddToCart.Topic(),
			event.TypeBehaviorOrderCreate.Topic(),
			event.TypeBehaviorOrderPay.Topic(),
			event.TypeBehaviorOrderCancel.Topic(),
			event.TypeBehaviorOrderRefund.Topic(),
		}
	}
	qualified := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		qualifiedTopic := qualifyTopic(prefix, topic)
		if qualifiedTopic == "" {
			continue
		}
		if _, ok := seen[qualifiedTopic]; ok {
			continue
		}
		seen[qualifiedTopic] = struct{}{}
		qualified = append(qualified, qualifiedTopic)
	}
	return qualified
}

func cloneHeaders(headers []kafkago.Header) []kafkago.Header {
	out := make([]kafkago.Header, len(headers))
	copy(out, headers)
	return out
}
