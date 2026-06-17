// Package outbox implements the background relay that polls the outbox table
// and publishes pending events through the configured event.Publisher.
package outbox

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// Publisher polls an OutboxStore and forwards pending events to a Publisher.
type Publisher struct {
	store      event.OutboxStore
	publisher  event.Publisher
	interval   time.Duration
	batchSize  int
	logger     *log.Logger
	stop       chan struct{}
	stopped    chan struct{}
}

// Config configures the outbox publisher relay.
type Config struct {
	Interval  time.Duration
	BatchSize int
	Logger    *log.Logger
}

// NewPublisher returns a relay that is not yet running. Call Start to begin
// polling and Stop to shut it down gracefully.
func NewPublisher(store event.OutboxStore, publisher event.Publisher, cfg Config) *Publisher {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Publisher{
		store:     store,
		publisher: publisher,
		interval:  cfg.Interval,
		batchSize: cfg.BatchSize,
		logger:    cfg.Logger,
		stop:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// Start begins the background polling loop. It returns immediately.
func (p *Publisher) Start() {
	go p.loop()
}

// Stop signals the loop to exit and waits for it to finish.
func (p *Publisher) Stop() {
	close(p.stop)
	<-p.stopped
}

func (p *Publisher) loop() {
	defer close(p.stopped)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Run once immediately on start.
	p.tick(context.Background())

	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.tick(context.Background())
		}
	}
}

func (p *Publisher) tick(ctx context.Context) {
	events, err := p.store.PollPending(ctx, p.batchSize)
	if err != nil {
		p.logger.Printf("outbox poll failed: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	published := make([]int64, 0, len(events))
	for _, evt := range events {
		if err := p.publisher.Publish(ctx, evt); err != nil {
			p.logger.Printf("outbox publish failed for event %d: %v", evt.ID, err)
			if markErr := p.store.MarkFailed(ctx, evt.ID, fmt.Sprintf("publish: %v", err)); markErr != nil {
				p.logger.Printf("outbox mark failed for event %d: %v", evt.ID, markErr)
			}
			continue
		}
		published = append(published, evt.ID)
	}

	if len(published) > 0 {
		if err := p.store.MarkPublished(ctx, published); err != nil {
			p.logger.Printf("outbox mark published failed: %v", err)
		}
	}
}
