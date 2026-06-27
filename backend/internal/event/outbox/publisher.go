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

type Publisher struct {
	store     event.OutboxRelayStore
	publisher event.Publisher
	interval  time.Duration
	batchSize int
	logger    *log.Logger
	stop      chan struct{}
	stopped   chan struct{}
}

type Config struct {
	Interval  time.Duration
	BatchSize int
	Logger    *log.Logger
}

func NewPublisher(store event.OutboxRelayStore, publisher event.Publisher, cfg Config) *Publisher {
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

func (p *Publisher) Start() {
	go p.loop()
}

func (p *Publisher) Stop() {
	close(p.stop)
	<-p.stopped
}

func (p *Publisher) loop() {
	defer close(p.stopped)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

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
	tx, err := p.store.BeginTx(ctx)
	if err != nil {
		p.logger.Printf("outbox begin tx failed: %v", err)
		return
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	events, err := p.store.PollPendingInTx(ctx, tx, p.batchSize)
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
			if markErr := p.store.MarkFailedInTx(ctx, tx, evt.ID, fmt.Sprintf("publish: %v", err)); markErr != nil {
				p.logger.Printf("outbox mark failed for event %d: %v", evt.ID, markErr)
			}
			continue
		}
		published = append(published, evt.ID)
	}

	if len(published) > 0 {
		if err := p.store.MarkPublishedInTx(ctx, tx, published); err != nil {
			p.logger.Printf("outbox mark published failed: %v", err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		p.logger.Printf("outbox tx commit failed: %v", err)
		return
	}
	committed = true
}
