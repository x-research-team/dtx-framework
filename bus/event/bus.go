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
	opts     busOptions[T]
}

// NewBus создает новый, строго типизированный экземпляр Bus для конкретного
// типа события T и связанного с ним топика.
func NewBus[T Event](topic string, provider Provider[T], opts ...BusOption[T]) (*busImpl[T], error) {
	if topic == "" {
		return nil, fmt.Errorf("topic не может быть пустым")
	}
	if provider == nil {
		return nil, fmt.Errorf("provider не может быть nil")
	}

	options := busOptions[T]{}
	for _, opt := range opts {
		opt(&options)
	}

	return &busImpl[T]{
		topic:    topic,
		provider: provider,
		opts:     options,
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

// busOptions содержит конфигурацию для шины.
type busOptions[T Event] struct {
	// Здесь могут быть опции, специфичные для шины, например, метрики или логгер.
}

// BusOption - это функция для настройки Bus.
type BusOption[T Event] func(*busOptions[T])
