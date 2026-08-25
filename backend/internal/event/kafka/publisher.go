// Package kafka provides Kafka-backed event publisher and consumer adapters.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	kafkago "github.com/segmentio/kafka-go"
)

const publishTimeout = 10 * time.Second

// PublisherConfig configures the Kafka publisher. TopicPrefix namespaces the
// logical event topic, e.g. redcart.events + order.paid => redcart.events.order.paid.
type PublisherConfig struct {
	TopicPrefix  string
	BatchTimeout time.Duration
	Logger       *log.Logger
}

// Publisher implements event.Publisher using Kafka topics.
type Publisher struct {
	writer      *kafkago.Writer
	topicPrefix string
}

// NewPublisher creates a Kafka publisher. The writer is synchronous and waits
// for all in-sync replicas before returning from Publish.
func NewPublisher(brokers []string, cfg PublisherConfig) (*Publisher, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.BatchTimeout <= 0 {
		cfg.BatchTimeout = 10 * time.Millisecond
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		Async:                  false,
		BatchTimeout:           cfg.BatchTimeout,
		ReadTimeout:            publishTimeout,
		WriteTimeout:           publishTimeout,
		AllowAutoTopicCreation: true,
		ErrorLogger:            logger,
	}
	return &Publisher{writer: writer, topicPrefix: cfg.TopicPrefix}, nil
}

// Publish serializes an event envelope and writes it to the event's Kafka topic.
func (p *Publisher) Publish(ctx context.Context, evt event.Event) error {
	body, err := encodeEvent(evt)
	if err != nil {
		return err
	}
	logicalTopic := evt.Topic
	if logicalTopic == "" {
		logicalTopic = evt.Type.Topic()
	}
	topic := qualifyTopic(p.topicPrefix, logicalTopic)
	if topic == "" {
		return fmt.Errorf("kafka topic is empty for event %d", evt.ID)
	}

	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(strconv.FormatInt(evt.ID, 10)),
		Value: body,
		Time:  evt.OccurredAt,
		Headers: []kafkago.Header{
			{Key: "event_type", Value: []byte(evt.Type)},
			{Key: "event_topic", Value: []byte(logicalTopic)},
			{Key: "correlation_id", Value: []byte(evt.CorrelationID)},
		},
	}
	if err := p.writer.WriteMessages(publishCtx, msg); err != nil {
		return fmt.Errorf("publish to kafka topic %s: %w", topic, err)
	}
	return nil
}

// Close flushes buffered Kafka writes and closes the writer.
func (p *Publisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

// ParseBrokers parses a comma-separated KAFKA_BROKERS value.
func ParseBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			brokers = append(brokers, part)
		}
	}
	return brokers
}

func encodeEvent(evt event.Event) ([]byte, error) {
	body, err := json.Marshal(map[string]any{
		"event_id":       evt.ID,
		"event_type":     string(evt.Type),
		"topic":          evt.Topic,
		"correlation_id": evt.CorrelationID,
		"occurred_at":    evt.OccurredAt,
		"payload":        evt.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	return body, nil
}

func qualifyTopic(prefix, topic string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	topic = strings.Trim(strings.TrimSpace(topic), ".")
	if prefix == "" || topic == "" || strings.HasPrefix(topic, prefix+".") {
		return topic
	}
	return prefix + "." + topic
}
