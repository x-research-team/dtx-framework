package event

import (
	"context"
	"fmt"
)


// Bus — это легковесный, потокобезопасный фасад для системы событий.
// Он делегирует всю работу по публикации, подписке и управлению жизненным циклом
// нижележащему провайдеру (Provider), реализуя паттерн "Стратегия".
// Это позволяет легко заменять транспортный уровень (например, с in-memory на Kafka)
// без изменения клиентского кода.
type Bus[T Event] struct {
	provider Provider
	opts     busOptions
}

// NewBus создает новый экземпляр Bus.
// Принимает функциональные опции для гибкой настройки.
// Если провайдер не указан явно через опцию WithProvider,
// по умолчанию создается и используется LocalProvider.
func NewBus(opts ...BusOption) (*Bus[Event], error) {
	options := busOptions{
		// Устанавливаем значения по умолчанию
		workerMin: 1,
		workerMax: 10,
		queueSize: 100,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if options.logger == nil {
		options.logger = &noopLogger{}
	}
	if options.metrics == nil {
		options.metrics = &noopMetrics{}
	}

	b := &Bus[Event]{
		opts: options,
	}

	// Если провайдер не был предоставлен через опции,
	// создаем и используем локальный провайдер по умолчанию.
	if options.provider == nil {
		// Создаем и используем локальный провайдер по умолчанию.
		localProvider, err := NewLocalProvider(options)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания локального провайдера по умолчанию: %w", err)
		}
		b.provider = localProvider
	} else {
		b.provider = options.provider
	}

	return b, nil
}

// Publish делегирует публикацию события текущему провайдеру.
func (b *Bus[T]) Publish(ctx context.Context, event Event) error {
	return b.provider.Publish(ctx, event)
}

// Subscribe делегирует подписку на события текущему провайдеру.
func (b *Bus[T]) Subscribe(topic string, handler any, opts ...SubscribeOption[Event]) (unsubscribe func(), err error) {
	return b.provider.Subscribe(topic, handler, opts...)
}

// Shutdown делегирует корректное завершение работы текущему провайдеру.
func (b *Bus[T]) Shutdown(ctx context.Context) error {
	return b.provider.Shutdown(ctx)
}
