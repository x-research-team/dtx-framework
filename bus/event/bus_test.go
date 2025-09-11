package event

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// --- Тестовые события ---

type UserCreatedEvent struct {
	UserID string
	Email  string
	meta   map[string]string
}

func (e *UserCreatedEvent) Metadata() map[string]string {
	if e.meta == nil {
		e.meta = make(map[string]string)
	}
	return e.meta
}

type OrderPaidEvent struct {
	OrderID string
	Amount  float64
	meta    map[string]string
}

func (e *OrderPaidEvent) Metadata() map[string]string {
	if e.meta == nil {
		e.meta = make(map[string]string)
	}
	return e.meta
}

// --- Тесты ---

func TestRegistry_Bus(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	topic := "test.topic"

	t.Run("получение шины в первый раз", func(t *testing.T) {
		t.Parallel()
		bus, err := Bus[*UserCreatedEvent](registry, topic)
		require.NoError(t, err)
		require.NotNil(t, bus)
	})

	t.Run("получение существующей шины", func(t *testing.T) {
		t.Parallel()
		bus1, err1 := Bus[*UserCreatedEvent](registry, topic)
		require.NoError(t, err1)

		bus2, err2 := Bus[*UserCreatedEvent](registry, topic)
		require.NoError(t, err2)

		assert.Same(t, bus1, bus2, "должен возвращаться один и тот же экземпляр шины")
	})

	t.Run("ошибка при получении шины с другим типом для того же топика", func(t *testing.T) {
		t.Parallel()
		_, err := Bus[*UserCreatedEvent](registry, "topic.conflict")
		require.NoError(t, err)

		_, err = Bus[*OrderPaidEvent](registry, "topic.conflict")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "уже существует с другим типом события")
	})
}

func TestBus_PublishSubscribe_Sync(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.sync")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedEvent *UserCreatedEvent
	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		receivedEvent = e
		wg.Done()
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler)
	require.NoError(t, err)
	defer unsubscribe()

	event := &UserCreatedEvent{UserID: "user-sync-123", Email: "sync@test.com"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()
	assert.Equal(t, event, receivedEvent)
}

func TestBus_PublishSubscribe_Async(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[*OrderPaidEvent](registry, "order.paid.async")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedEvent *OrderPaidEvent
	handler := func(ctx context.Context, e *OrderPaidEvent) error {
		time.Sleep(10 * time.Millisecond) // имитация работы
		receivedEvent = e
		wg.Done()
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler, WithAsync[*OrderPaidEvent]())
	require.NoError(t, err)
	defer unsubscribe()

	event := &OrderPaidEvent{OrderID: "order-async-456", Amount: 99.99}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()
	assert.Equal(t, event, receivedEvent)
}

func TestBus_Middleware(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.middleware")
	require.NoError(t, err)

	var middleware1Called, middleware2Called, handlerCalled bool

	mw1 := func(next EventHandler[*UserCreatedEvent]) EventHandler[*UserCreatedEvent] {
		return func(ctx context.Context, e *UserCreatedEvent) error {
			middleware1Called = true
			return next(ctx, e)
		}
	}

	mw2 := func(next EventHandler[*UserCreatedEvent]) EventHandler[*UserCreatedEvent] {
		return func(ctx context.Context, e *UserCreatedEvent) error {
			middleware2Called = true
			return next(ctx, e)
		}
	}

	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		handlerCalled = true
		return nil
	}

	unsubscribe, err := bus.Subscribe(handler, WithMiddleware(mw1, mw2))
	require.NoError(t, err)
	defer unsubscribe()

	err = bus.Publish(context.Background(), &UserCreatedEvent{})
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
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.error")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2) // Ожидаем вызов обработчика и обработчика ошибок

	handlerErr := fmt.Errorf("ошибка в обработчике")
	var receivedErr error
	var receivedEvent *UserCreatedEvent

	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		wg.Done()
		return handlerErr
	}

	errorHandler := func(err error, e *UserCreatedEvent) {
		receivedErr = err
		receivedEvent = e
		wg.Done()
	}

	unsubscribe, err := bus.Subscribe(handler, WithErrorHandler(errorHandler))
	require.NoError(t, err)
	defer unsubscribe()

	event := &UserCreatedEvent{UserID: "user-error-789"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, handlerErr, receivedErr, "обработчик ошибок должен получить правильную ошибку")
	assert.Equal(t, event, receivedEvent, "обработчик ошибок должен получить правильное событие")
}

func TestRegistry_Shutdown(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	_, err := Bus[*UserCreatedEvent](registry, "shutdown.test.1")
	require.NoError(t, err)
	_, err = Bus[*OrderPaidEvent](registry, "shutdown.test.2")
	require.NoError(t, err)

	err = registry.Shutdown(context.Background())
	require.NoError(t, err)

	// Проверяем, что пул воркеров остановлен (косвенно)
	// Попытка опубликовать событие после Shutdown не должна вызывать панику.
	// В реальной системе провайдер мог бы возвращать ошибку.
	bus, _ := Bus[*UserCreatedEvent](registry, "shutdown.test.1")
	err = bus.Publish(context.Background(), &UserCreatedEvent{})
	assert.NoError(t, err) // Наш noop-like shutdown не возвращает ошибок
}

// --- Тесты Observability ---

func TestBus_WithLogging(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.logging", WithLogger[*UserCreatedEvent](logger))
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		wg.Done()
		return nil
	}
	_, err = bus.Subscribe(handler)
	require.NoError(t, err)

	event := &UserCreatedEvent{UserID: "user-log-123", Email: "log@test.com"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, `"msg":"публикация события"`)
	assert.Contains(t, logOutput, `"event_type":"UserCreatedEvent"`)
	assert.Contains(t, logOutput, `"msg":"начало обработки события"`)
	assert.Contains(t, logOutput, `"msg":"событие успешно обработано"`)
	assert.Contains(t, logOutput, `"handler_name"`)
}

func TestBus_WithMetrics(t *testing.T) {
	t.Parallel()

	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.metrics", WithMeterProvider[*UserCreatedEvent](meterProvider))
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		time.Sleep(5 * time.Millisecond)
		wg.Done()
		return nil
	}
	_, err = bus.Subscribe(handler)
	require.NoError(t, err)

	event := &UserCreatedEvent{UserID: "user-metrics-123"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()

	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	require.Len(t, rm.ScopeMetrics, 1, "должна быть одна группа метрик")
	scopeMetrics := rm.ScopeMetrics[0]
	require.Len(t, scopeMetrics.Metrics, 3, "должно быть три метрики")

	// Проверка счетчика публикации
	publishCount := findMetric(t, scopeMetrics.Metrics, "messaging.publish.count")
	sum := publishCount.Data.(metricdata.Sum[int64])
	assert.Equal(t, int64(1), sum.DataPoints[0].Value)
	assert.True(t, hasAttribute(sum.DataPoints[0].Attributes, "event.type", "UserCreatedEvent"))
	assert.True(t, hasAttribute(sum.DataPoints[0].Attributes, "status", "success"))

	// Проверка счетчика обработки
	consumeCount := findMetric(t, scopeMetrics.Metrics, "messaging.consume.count")
	sum = consumeCount.Data.(metricdata.Sum[int64])
	assert.Equal(t, int64(1), sum.DataPoints[0].Value)
	assert.True(t, hasAttribute(sum.DataPoints[0].Attributes, "event.type", "UserCreatedEvent"))
	assert.True(t, hasAttribute(sum.DataPoints[0].Attributes, "status", "success"))

	// Проверка гистограммы длительности
	consumeDuration := findMetric(t, scopeMetrics.Metrics, "messaging.consume.duration")
	hist := consumeDuration.Data.(metricdata.Histogram[float64])
	assert.Equal(t, uint64(1), hist.DataPoints[0].Count)
	assert.Greater(t, hist.DataPoints[0].Sum, 0.0)
}

func TestBus_WithTracing(t *testing.T) {
	t.Parallel()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.tracing", WithTracerProvider[*UserCreatedEvent](tracerProvider))
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		// Проверяем, что в обработчик пришел активный спан
		span := oteltrace.SpanFromContext(ctx)
		require.True(t, span.IsRecording())
		wg.Done()
		return nil
	}
	_, err = bus.Subscribe(handler)
	require.NoError(t, err)

	event := &UserCreatedEvent{UserID: "user-trace-123"}
	err = bus.Publish(context.Background(), event)
	require.NoError(t, err)

	wg.Wait()

	// Проверяем записанные спаны
	spans := spanRecorder.Ended()
	require.Len(t, spans, 2, "должно быть два спана: publish и process")

	var producerSpan, consumerSpan trace.ReadOnlySpan
	for _, s := range spans {
		if s.SpanKind() == oteltrace.SpanKindProducer {
			producerSpan = s
		}
		if s.SpanKind() == oteltrace.SpanKindConsumer {
			consumerSpan = s
		}
	}

	require.NotNil(t, producerSpan, "не найден спан Producer")
	require.NotNil(t, consumerSpan, "не найден спан Consumer")

	assert.Equal(t, "UserCreatedEvent publish", producerSpan.Name())
	assert.Equal(t, "UserCreatedEvent process", consumerSpan.Name())
	assert.Equal(t, producerSpan.SpanContext().TraceID(), consumerSpan.SpanContext().TraceID())
	assert.Equal(t, producerSpan.SpanContext().SpanID(), consumerSpan.Parent().SpanID())
}

func TestBus_ContextPropagation(t *testing.T) {
	t.Parallel()

	type contextKey string
	const testKey = contextKey("test-value")
	const testValue = "propagated-successfully"

	registry := NewRegistry()
	bus, err := Bus[*UserCreatedEvent](registry, "user.created.context")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedValue string
	handler := func(ctx context.Context, e *UserCreatedEvent) error {
		if val, ok := ctx.Value(testKey).(string); ok {
			receivedValue = val
		}
		wg.Done()
		return nil
	}

	// Используем асинхронную подписку, чтобы убедиться, что контекст
	// корректно передается между горутинами.
	unsubscribe, err := bus.Subscribe(handler, WithAsync[*UserCreatedEvent]())
	require.NoError(t, err)
	defer unsubscribe()

	ctx := context.WithValue(context.Background(), testKey, testValue)
	event := &UserCreatedEvent{UserID: "user-ctx-123"}
	err = bus.Publish(ctx, event)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, testValue, receivedValue, "значение из контекста должно быть успешно передано в обработчик")
}

// --- Вспомогательные функции для тестов ---

func findMetric(t *testing.T, metrics []metricdata.Metrics, name string) metricdata.Metrics {
	t.Helper()
	for _, m := range metrics {
		if m.Name == name {
			return m
		}
	}
	require.Fail(t, "метрика не найдена", "имя метрики: %s", name)
	return metricdata.Metrics{}
}

func hasAttribute(set attribute.Set, key, value string) bool {
	val, ok := set.Value(attribute.Key(key))
	return ok && val.AsString() == value
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
	var zero T
	// Мы должны создать не-nil значение для типа T, который является указателем.
	// reflect.New(reflect.TypeOf(zero).Elem()) создает указатель на новый нулевой объект.
	// Например, если T это *UserCreatedEvent, это создаст нового &UserCreatedEvent{}.
	event := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(T)

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

func BenchmarkPublish_Sync_OneSubscriber(b *testing.B) {
	benchmarkPublish[*UserCreatedEvent](b, "bench.sync.one", 1, false)
}

func BenchmarkPublish_Sync_MultipleSubscribers(b *testing.B) {
	benchmarkPublish[*UserCreatedEvent](b, "bench.sync.multiple", 100, false)
}

func BenchmarkPublish_Async_OneSubscriber(b *testing.B) {
	benchmarkPublish[*OrderPaidEvent](b, "bench.async.one", 1, true)
}

func BenchmarkPublish_Async_MultipleSubscribers(b *testing.B) {
	benchmarkPublish[*OrderPaidEvent](b, "bench.async.multiple", 100, true)
}
