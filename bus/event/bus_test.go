package event

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Тестовые события ---

type UserCreatedEvent struct {
	UserID string
	Email  string
}

type OrderPaidEvent struct {
	OrderID string
	Amount  float64
}

// --- Тесты ---

func TestRegistry_Bus(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	topic := "test.topic"

	t.Run("получение шины в первый раз", func(t *testing.T) {
		t.Parallel()
		bus, err := Bus[UserCreatedEvent](registry, topic)
		require.NoError(t, err)
		require.NotNil(t, bus)
	})

	t.Run("получение существующей шины", func(t *testing.T) {
		t.Parallel()
		bus1, err1 := Bus[UserCreatedEvent](registry, topic)
		require.NoError(t, err1)

		bus2, err2 := Bus[UserCreatedEvent](registry, topic)
		require.NoError(t, err2)

		assert.Same(t, bus1, bus2, "должен возвращаться один и тот же экземпляр шины")
	})

	t.Run("ошибка при получении шины с другим типом для того же топика", func(t *testing.T) {
		t.Parallel()
		_, err := Bus[UserCreatedEvent](registry, "topic.conflict")
		require.NoError(t, err)

		_, err = Bus[OrderPaidEvent](registry, "topic.conflict")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "уже существует с другим типом события")
	})
}

func TestBus_PublishSubscribe_Sync(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[UserCreatedEvent](registry, "user.created.sync")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedEvent UserCreatedEvent
	handler := func(ctx context.Context, e UserCreatedEvent) error {
		receivedEvent = e
		wg.Done()
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler)
	require.NoError(t, err)
	defer unsubscribe()

	event := UserCreatedEvent{UserID: "user-sync-123", Email: "sync@test.com"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()
	assert.Equal(t, event, receivedEvent)
}

func TestBus_PublishSubscribe_Async(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[OrderPaidEvent](registry, "order.paid.async")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedEvent OrderPaidEvent
	handler := func(ctx context.Context, e OrderPaidEvent) error {
		time.Sleep(10 * time.Millisecond) // имитация работы
		receivedEvent = e
		wg.Done()
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler, WithAsync[OrderPaidEvent]())
	require.NoError(t, err)
	defer unsubscribe()

	event := OrderPaidEvent{OrderID: "order-async-456", Amount: 99.99}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()
	assert.Equal(t, event, receivedEvent)
}

func TestBus_Middleware(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[UserCreatedEvent](registry, "user.created.middleware")
	require.NoError(t, err)

	var middleware1Called, middleware2Called, handlerCalled bool

	mw1 := func(next EventHandler[UserCreatedEvent]) EventHandler[UserCreatedEvent] {
		return func(ctx context.Context, e UserCreatedEvent) error {
			middleware1Called = true
			return next(ctx, e)
		}
	}

	mw2 := func(next EventHandler[UserCreatedEvent]) EventHandler[UserCreatedEvent] {
		return func(ctx context.Context, e UserCreatedEvent) error {
			middleware2Called = true
			return next(ctx, e)
		}
	}

	handler := func(ctx context.Context, e UserCreatedEvent) error {
		handlerCalled = true
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler, WithMiddleware(mw1, mw2))
	require.NoError(t, err)
	defer unsubscribe()

	err = bus.Publish(context.Background(), UserCreatedEvent{})
	require.NoError(t, err)

	// Даем время на выполнение в случае асинхронности (хотя здесь синхронно)
	time.Sleep(20 * time.Millisecond)

	assert.True(t, middleware1Called, "первый middleware должен быть вызван")
	assert.True(t, middleware2Called, "второй middleware должен быть вызван")
	assert.True(t, handlerCalled, "основной обработчик должен быть вызван")
}

func TestBus_ErrorHandler(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[UserCreatedEvent](registry, "user.created.error")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2) // Ожидаем вызов обработчика и обработчика ошибок

	handlerErr := fmt.Errorf("ошибка в обработчике")
	var receivedErr error
	var receivedEvent UserCreatedEvent

	handler := func(ctx context.Context, e UserCreatedEvent) error {
		wg.Done()
		return handlerErr
	}

	errorHandler := func(err error, e UserCreatedEvent) {
		receivedErr = err
		receivedEvent = e
		wg.Done()
	}

	unsubscribe, err := bus.Subscribe(handler, WithErrorHandler(errorHandler))
	require.NoError(t, err)
	defer unsubscribe()

	event := UserCreatedEvent{UserID: "user-error-789"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, handlerErr, receivedErr, "обработчик ошибок должен получить правильную ошибку")
	assert.Equal(t, event, receivedEvent, "обработчик ошибок должен получить правильное событие")
}

func TestRegistry_Shutdown(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	_, err := Bus[UserCreatedEvent](registry, "shutdown.test.1")
	require.NoError(t, err)
	_, err = Bus[OrderPaidEvent](registry, "shutdown.test.2")
	require.NoError(t, err)

	err = registry.Shutdown(context.Background())
	require.NoError(t, err)

	// Проверяем, что пул воркеров остановлен (косвенно)
	// Попытка опубликовать событие после Shutdown не должна вызывать панику.
	// В реальной системе провайдер мог бы возвращать ошибку.
	bus, _ := Bus[UserCreatedEvent](registry, "shutdown.test.1")
	err = bus.Publish(context.Background(), UserCreatedEvent{})
	assert.NoError(t, err) // Наш noop-like shutdown не возвращает ошибок
}

// --- Тесты производительности ---

// benchmarkPublish — это обобщенная вспомогательная функция для создания
// и запуска тестов производительности для шины событий. Она позволяет тестировать
// различные сценарии (синхронные/асинхронные, один/много подписчиков)
// без дублирования кода.
func benchmarkPublish[T Event](b *testing.B, topic string, numSubscribers int, async bool) {
	registry := NewRegistry()
	bus, err := Bus[T](registry, topic)
	if err != nil {
		b.Fatalf("не удалось получить шину: %v", err)
	}

	var wg sync.WaitGroup
	if async {
		// Для асинхронных тестов нам нужно дождаться завершения всех обработчиков,
		// чтобы гарантировать, что вся работа выполнена в рамках измерения.
		wg.Add(numSubscribers * b.N)
	}

	handler := func(ctx context.Context, e T) error {
		if async {
			wg.Done()
		}
		return nil
	}

	var opts []SubscribeOption[T]
	if async {
		opts = append(opts, WithAsync[T]())
	}

	for i := 0; i < numSubscribers; i++ {
		// Создаем уникальное имя подписчика, чтобы избежать конфликтов
		subscriberName := fmt.Sprintf("subscriber-%d", i)
		finalOpts := append(opts, WithSubscriberName[T](subscriberName))
		unsubscribe, err := bus.Subscribe(handler, finalOpts...)
		if err != nil {
			b.Fatalf("не удалось подписаться: %v", err)
		}
		defer unsubscribe()
	}

	// Создаем одно событие для переиспользования во всех итерациях,
	// чтобы избежать накладных расходов на аллокацию в цикле измерения.
	var event T

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(context.Background(), event); err != nil {
				b.Errorf("ошибка публикации: %v", err)
			}
		}
	})

	if async {
		wg.Wait()
	}
}

func TestBus_WithProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	topic := "test.with.provider"

	// 1. Создаем mock-провайдер
	mockProvider := &mockProvider[UserCreatedEvent]{
		publishFunc: func(ctx context.Context, event UserCreatedEvent) error {
			// Проверяем, что событие дошло до нашего мока
			assert.Equal(t, "user-provider-123", event.UserID)
			return nil
		},
		subscribeFunc: func(handler EventHandler[UserCreatedEvent], opts ...SubscribeOption[UserCreatedEvent]) (unsubscribe func(), err error) {
			// Для этого теста нам не нужно реализовывать подписку в моке
			return func() {}, nil
		},
	}

	// 2. Получаем шину с опцией WithProvider
	bus, err := Bus[UserCreatedEvent](registry, topic, WithProvider[UserCreatedEvent](mockProvider))
	require.NoError(t, err)
	require.NotNil(t, bus)

	// 3. Публикуем событие
	event := UserCreatedEvent{UserID: "user-provider-123", Email: "provider@test.com"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	// 4. Проверяем, что метод Publish был вызван у мока
	assert.True(t, mockProvider.publishCalled, "метод Publish у mock-провайдера должен быть вызван")
}

// --- Mock Provider ---

// mockProvider - это mock-реализация интерфейса Provider для тестирования.
type mockProvider[T Event] struct {
	publishFunc     func(ctx context.Context, event T) error
	subscribeFunc   any
	shutdownFunc    func(ctx context.Context) error
	publishCalled   bool
	subscribeCalled bool
	shutdownCalled  bool
}

func (m *mockProvider[T]) Publish(ctx context.Context, event T) error {
	m.publishCalled = true
	if m.publishFunc != nil {
		return m.publishFunc(ctx, event)
	}
	return nil
}

func (m *mockProvider[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	m.subscribeCalled = true
	if m.subscribeFunc != nil {
		// Используем приведение типа к конкретной функции, сохраненной в `any`.
		// Это правильный способ работы с обобщенными типами в моках.
		if fn, ok := m.subscribeFunc.(func(EventHandler[T], ...SubscribeOption[T]) (func(), error)); ok {
			return fn(handler, opts...)
		}
	}
	return func() {}, nil
}

func (m *mockProvider[T]) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx)
	}
	return nil
}

func BenchmarkPublish_Sync_OneSubscriber(b *testing.B) {
	benchmarkPublish[UserCreatedEvent](b, "bench.sync.one", 1, false)
}

func BenchmarkPublish_Sync_MultipleSubscribers(b *testing.B) {
	benchmarkPublish[UserCreatedEvent](b, "bench.sync.multiple", 100, false)
}

func BenchmarkPublish_Async_OneSubscriber(b *testing.B) {
	benchmarkPublish[OrderPaidEvent](b, "bench.async.one", 1, true)
}

func BenchmarkPublish_Async_MultipleSubscribers(b *testing.B) {
	benchmarkPublish[OrderPaidEvent](b, "bench.async.multiple", 100, true)
}
