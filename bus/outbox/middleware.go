package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/x-research-team/dtx-framework/bus/event"
)

// NewOutboxMiddleware создает новый экземпляр OutboxMiddleware.
func NewOutboxMiddleware[T event.Event](storage Storage[T], opts ...Option[T]) *OutboxMiddleware[T] {
	m := &OutboxMiddleware[T]{
		storage: storage,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// OutboxMiddleware реализует паттерн Transactional Outbox.
type OutboxMiddleware[T event.Event] struct {
	storage Storage[T]
	topic   string
}

// Wrap оборачивает провайдер, перенаправляя публикацию в Storage.
func (m *OutboxMiddleware[T]) Wrap(next event.Provider[T]) event.Provider[T] {
	return &outboxProvider[T]{
		storage: m.storage,
		next:    next,
		topic:   m.topic,
	}
}

// outboxProvider - это реализация провайдера, которая пишет в Storage.
type outboxProvider[T event.Event] struct {
	storage Storage[T]
	next    event.Provider[T]
	topic   string
}

// Publish сохраняет событие в Storage вместо реальной отправки.
func (p *outboxProvider[T]) Publish(ctx context.Context, event T) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := &Message{
		ID:        uuid.New(),
		Topic:     p.topic,
		Payload:   payload,
		Metadata:  event.Metadata(),
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	return p.storage.Save(ctx, msg)
}

// Subscribe делегирует вызов следующему провайдеру в цепочке.
func (p *outboxProvider[T]) Subscribe(handler event.EventHandler[T], opts ...event.SubscribeOption[T]) (unsubscribe func(), err error) {
	return p.next.Subscribe(handler, opts...)
}

// Shutdown делегирует вызов следующему провайдеру в цепочке.
func (p *outboxProvider[T]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}
