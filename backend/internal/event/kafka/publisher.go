// Package kafka adapts Kafka to the broker-neutral event.Publisher contract.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/twmb/franz-go/pkg/kgo"
)

const connectTimeout = 5 * time.Second

// producer is the small part of franz-go used by Publisher. The interface
// keeps tests local and makes the broker acknowledgement boundary explicit.
type producer interface {
	ProduceSync(context.Context, ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Publisher sends every business event to one Kafka topic. event.Topic stays
// in the JSON envelope so consumers can route order.created, order.paid, etc.
type Publisher struct {
	producer producer
	topic    string
}

// NewPublisher creates a Kafka producer and verifies that a broker is
// reachable. franz-go enables idempotent production by default; ProduceSync
// waits for the broker acknowledgement before the outbox row is marked done.
func NewPublisher(brokers []string, topic string) (*Publisher, error) {
	brokers = cleanBrokers(brokers)
	topic = strings.TrimSpace(topic)
	if len(brokers) == 0 {
		return nil, errors.New("at least one Kafka broker is required")
	}
	if topic == "" {
		return nil, errors.New("Kafka topic is required")
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("connect to Kafka: %w", err)
	}
	return newPublisher(client, topic), nil
}

func newPublisher(client producer, topic string) *Publisher {
	return &Publisher{producer: client, topic: topic}
}

func (p *Publisher) Publish(ctx context.Context, evt event.Event) error {
	value, err := encodeEvent(evt)
	if err != nil {
		return err
	}
	key := evt.CorrelationID
	if key == "" {
		key = strconv.FormatInt(evt.ID, 10)
	}
	record := &kgo.Record{
		Topic:     p.topic,
		Key:       []byte(key),
		Value:     value,
		Timestamp: evt.OccurredAt,
		Headers: []kgo.RecordHeader{
			{Key: "event_id", Value: []byte(strconv.FormatInt(evt.ID, 10))},
			{Key: "event_type", Value: []byte(evt.Type)},
			{Key: "event_topic", Value: []byte(evt.Topic)},
		},
	}
	if err := p.producer.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("Kafka publish acknowledgement for event %d: %w", evt.ID, err)
	}
	return nil
}

func (p *Publisher) Close() {
	p.producer.Close()
}

func cleanBrokers(brokers []string) []string {
	cleaned := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		if broker = strings.TrimSpace(broker); broker != "" {
			cleaned = append(cleaned, broker)
		}
	}
	return cleaned
}

var _ event.Publisher = (*Publisher)(nil)
