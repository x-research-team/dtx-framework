package event

import (
	"context"
	"fmt"
)

// IBus определяет строго типизированный интерфейс для публикации и подписки
// на события конкретного типа T.
type IBus[T Event] interface {
	// Publish публикует событие типа T в шину.
	// Метод является полностью типобезопасным на этапе компиляции.
	Publish(ctx context.Context, event T) error

	// Subscribe подписывает строго типизированный обработчик на события типа T.
	// Метод является полностью типобезопасным на этапе компиляции.
	Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error)

	// Shutdown корректно завершает работу шины.
	Shutdown(ctx context.Context) error
}

// busImpl - это реализация строго типизированной шины событий.
type busImpl[T Event] struct {
	topic    string
	provider Provider[T]
	cfg      *config[T]
}

// NewBus создает новый, строго типизированный экземпляр Bus для конкретного
// типа события T и связанного с ним топика.
func NewBus[T Event](topic string, opts ...Option[T]) (*busImpl[T], error) {
	if topic == "" {
		return nil, fmt.Errorf("topic не может быть пустым")
	}

	cfg := &config[T]{}
	for _, opt := range opts {
		opt(cfg)
	}

	// В будущем здесь будет логика выбора провайдера на основе конфигурации.
	// Пока что, для обратной совместимости и выполнения текущей задачи,
	// мы создаем LocalProvider по умолчанию.
	provider, err := NewLocalProvider(topic, cfg)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать локальный провайдер: %w", err)
	}

	// Применяем middleware. Сначала добавляем middleware по умолчанию, затем пользовательские.
	// Это позволяет пользователю переопределить или дополнить стандартное поведение.
	allMiddlewares := []BusMiddleware[T]{
		NewLoggingMiddleware[T](cfg.logger),
		NewMetricsMiddleware[T](cfg.meterProvider),
		NewTracingMiddleware[T](cfg.tracerProvider, cfg.propagator),
	}
	allMiddlewares = append(allMiddlewares, cfg.middlewares...)
	wrappedProvider := applyMiddlewares(provider, allMiddlewares...)

	return &busImpl[T]{
		topic:    topic,
		provider: wrappedProvider,
		cfg:      cfg,
	}, nil
}

// Publish публикует событие в шину.
func (b *busImpl[T]) Publish(ctx context.Context, event T) error {
	return b.provider.Publish(ctx, event)
}

// Subscribe подписывает обработчик на события.
func (b *busImpl[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	return b.provider.Subscribe(handler, opts...)
}

// Shutdown завершает работу шины.
func (b *busImpl[T]) Shutdown(ctx context.Context) error {
	return b.provider.Shutdown(ctx)
}

