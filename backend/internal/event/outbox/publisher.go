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

// Publisher 是 outbox 后台转发器：周期性轮询 outbox 表中未发布的事件，
// 通过注入的 event.Publisher 发布到消息队列，并在同一数据库事务内标记状态，
// 从而保证"发布成功才标记"的可靠性语义。
type Publisher struct {
	store     event.OutboxRelayStore
	publisher event.Publisher
	interval  time.Duration
	batchSize int
	logger    *log.Logger
	stop      chan struct{}
	stopped   chan struct{}
}

// Config 是 NewPublisher 的配置项；字段为零值时使用默认值
// （Interval 默认 5s，BatchSize 默认 100，Logger 默认 log.Default()）。
type Config struct {
	Interval  time.Duration
	BatchSize int
	Logger    *log.Logger
}

// NewPublisher 创建 outbox 转发器，并用默认值补齐缺失的配置项。
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

// Start 在后台 goroutine 中启动转发循环，调用后立即返回（非阻塞）。
func (p *Publisher) Start() {
	go p.loop()
}

// Stop 停止转发循环并等待后台 goroutine 退出，保证资源释放完成。
func (p *Publisher) Stop() {
	close(p.stop)
	<-p.stopped
}

// loop 是后台主循环：启动后立即执行一次 tick，之后按 interval 周期执行，
// 收到 stop 信号即退出。
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

// tick 执行一次完整的转发周期：开启事务 → 锁定并取出未发布事件 →
// 逐条发布（失败的标记为 failed，不阻断批次中的其他事件）→ 提交事务。
func (p *Publisher) tick(ctx context.Context) {
	tx, err := p.store.BeginTx(ctx)
	if err != nil {
		p.logger.Printf("outbox begin tx failed: %v", err)
		return
	}

	committed := false
	// 事务未提交时统一回滚，避免失败路径残留半完成的标记。
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
		// 单条发布失败不中断整个批次：记录失败原因后继续处理下一条，
		// 失败记录最终会进入死信表。
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
