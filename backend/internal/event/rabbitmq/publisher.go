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
	reconnectDelay = 3 * time.Second  // 断线后的重连等待间隔
	publishTimeout = 10 * time.Second // 等待 broker 发布确认（confirm）的超时
)

// Publisher 是 event.Publisher 的 RabbitMQ 实现：向 topic exchange 发布事件，
// 启用 publisher confirms 等待 broker 落盘确认，并在连接断开后自动重连。
type Publisher struct {
	addr     string
	exchange string
	logger   *log.Logger

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

// NewPublisher 建立到 RabbitMQ 的连接并完成 exchange 声明，
// 返回的 Publisher 可直接用于发布事件。
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

// connect 建立连接、打开 channel 并声明 topic exchange、开启 publisher
// confirms，随后启动断线监听 goroutine。
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

// watchClose 监听 channel 关闭事件（连接断开或服务端异常），一旦发生便
// 以固定间隔反复尝试重连，直到重连成功或 Close 被调用。
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

// checkConnection 校验当前 channel 是否可用：不可用时返回错误。
// 注意：它只做状态检测，不会主动建立或重建连接——断线后的重连由
// watchClose 的重连循环负责，发布前调用它只是尽早暴露不可用状态。
func (p *Publisher) checkConnection() error {
	if p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	return fmt.Errorf("rabbitmq channel unavailable")
}

// Publish 将事件序列化为固定 envelope（字段与 consumer 侧解码结构保持一致），
// 以持久化消息发布到事件对应的 topic，并等待 broker 的发布确认后才返回。
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

	if err := p.checkConnection(); err != nil {
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

// Close 标记关闭并释放 channel 与连接，之后重连循环会随之退出。
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
