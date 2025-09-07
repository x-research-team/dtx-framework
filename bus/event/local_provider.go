package event

import (
	"context"
	"sync"
)

// Provider определяет контракт для сменных механизмов доставки событий
// конкретного типа T.
type Provider[T Event] interface {
	// Publish публикует событие типа T.
	Publish(ctx context.Context, event T) error

	// Subscribe подписывает строго типизированный обработчик на события типа T.
	Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error)

	// Shutdown корректно завершает работу провайдера.
	Shutdown(ctx context.Context) error
}

// subscription хранит информацию о конкретной подписке.
type subscription[T Event] struct {
	handler      EventHandler[T]
	isAsync      bool
	errorHandler ErrorHandler[T]
}

// LocalProvider - это локальная, внутрипроцессная реализация провайдера событий.
type LocalProvider[T Event] struct {
	mu            sync.RWMutex
	subscriptions map[string]*subscription[T]
	topic         string
	pool          *workerPool[T]
}

// NewLocalProvider создает новый экземпляр локального провайдера.
func NewLocalProvider[T Event](topic string) (*LocalProvider[T], error) {
	// В реальном приложении конфигурация пула воркеров должна быть настраиваемой.
	pool := newWorkerPool[T](1, 10, 100)
	pool.run()

	return &LocalProvider[T]{
		subscriptions: make(map[string]*subscription[T]),
		topic:         topic,
		pool:          pool,
	}, nil
}

// Publish публикует событие всем подписчикам.
func (p *LocalProvider[T]) Publish(ctx context.Context, event T) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, sub := range p.subscriptions {
		task := &Task[T]{
			event:   event,
			handler: sub.handler,
		}
		if sub.isAsync {
			p.pool.enqueue(task)
		} else {
			// Синхронное выполнение
			err := task.handler(ctx, task.event)
			if err != nil && sub.errorHandler != nil {
				sub.errorHandler(err, task.event)
			}
		}
	}
	return nil
}

// Subscribe подписывает обработчик на события.
func (p *LocalProvider[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	subOpts := subscriptionOptions[T]{}
	for _, opt := range opts {
		opt(&subOpts)
	}

	finalHandler := handler
	for i := len(subOpts.middleware) - 1; i >= 0; i-- {
		finalHandler = subOpts.middleware[i](finalHandler)
	}

	// Простой уникальный идентификатор для подписки.
	// В реальной системе может потребоваться более надежный механизм.
	subID := "sub_" + string(rune(len(p.subscriptions)))

	p.subscriptions[subID] = &subscription[T]{
		handler:      finalHandler,
		isAsync:      subOpts.isAsync,
		errorHandler: subOpts.errorHandler,
	}

	unsubscribe = func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.subscriptions, subID)
	}

	return unsubscribe, nil
}

// Shutdown завершает работу провайдера.
func (p *LocalProvider[T]) Shutdown(ctx context.Context) error {
	p.pool.stop()
	return nil
}

// subscriptionOptions содержит конфигурацию для подписки.
type subscriptionOptions[T Event] struct {
	name         string
	isAsync      bool
	errorHandler ErrorHandler[T]
	middleware   []Middleware[T]
}

// SubscribeOption - это функция для настройки подписки.
type SubscribeOption[T Event] func(*subscriptionOptions[T])

// WithMiddleware добавляет промежуточное ПО к обработчику события.
func WithMiddleware[T Event](mw ...Middleware[T]) SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.middleware = append(o.middleware, mw...)
	}
}

// WithErrorHandler устанавливает обработчик ошибок для подписки.
func WithErrorHandler[T Event](h ErrorHandler[T]) SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.errorHandler = h
	}
}

// WithAsync делает подписку асинхронной.
func WithAsync[T Event]() SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.isAsync = true
	}
}

// WithSubscriberName устанавливает уникальное имя для подписчика.
func WithSubscriberName[T Event](name string) SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.name = name
	}
}
