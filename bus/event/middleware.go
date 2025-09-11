package event

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/goccy/go-reflect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationName    = "github.com/x-research-team/dtx-framework/bus/event"
	instrumentationVersion = "0.1.0"
	metricKeyPrefix        = "messaging."
)

// BusMiddleware определяет интерфейс для middleware шины событий.
// Middleware позволяет добавлять сквозную функциональность, такую как логирование, метрики или трассировка,
// вокруг обработки событий.
type BusMiddleware[T Event] interface {
	// Wrap оборачивает следующий провайдер в цепочке, добавляя свою логику.
	Wrap(next Provider[T]) Provider[T]
}

// MiddlewareFunc является адаптером, позволяющим использовать обычные функции как middleware.
type MiddlewareFunc[T Event] func(next Provider[T]) Provider[T]

// Wrap реализует интерфейс BusMiddleware.
func (f MiddlewareFunc[T]) Wrap(next Provider[T]) Provider[T] {
	return f(next)
}

// loggingMiddleware реализует BusMiddleware для логирования операций с событиями.
type loggingMiddleware[T Event] struct {
	logger *slog.Logger
}

// NewLoggingMiddleware создает новое middleware для логирования.
// Если логгер не предоставлен (nil), возвращается no-op middleware.
func NewLoggingMiddleware[T Event](logger *slog.Logger) BusMiddleware[T] {
	if logger == nil {
		return &noopMiddleware[T]{}
	}
	return &loggingMiddleware[T]{
		logger: logger,
	}
}

// Wrap оборачивает провайдер для добавления логирования.
func (m *loggingMiddleware[T]) Wrap(next Provider[T]) Provider[T] {
	return &loggingProvider[T]{
		next:   next,
		logger: m.logger,
	}
}

// loggingProvider - это обертка над провайдером событий, которая добавляет логирование.
type loggingProvider[T Event] struct {
	next   Provider[T]
	logger *slog.Logger
}

// Publish логирует и публикует событие.
func (p *loggingProvider[T]) Publish(ctx context.Context, event T) (err error) {
	eventType, eventID := getEventTypeAndID(event)
	p.logger.Info("публикация события", slog.String("event_type", eventType), slog.String("event_id", eventID))

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if err != nil {
			p.logger.Error("ошибка публикации события",
				slog.String("event_type", eventType),
				slog.String("event_id", eventID),
				slog.Any("error", err),
				slog.Duration("duration", duration),
			)
		}
	}()

	return p.next.Publish(ctx, event)
}

// Subscribe логирует и подписывает обработчик на события.
func (p *loggingProvider[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	wrappedHandler := func(ctx context.Context, event T) (err error) {
		eventType, eventID := getEventTypeAndID(event)

		subOpts := &subscriptionOptions[T]{}
		for _, opt := range opts {
			opt(subOpts)
		}
		handlerName := subOpts.name
		if handlerName == "" {
			handlerName = getHandlerName(handler)
		}

		p.logger.Info("начало обработки события",
			slog.String("event_type", eventType),
			slog.String("event_id", eventID),
			slog.String("handler_name", handlerName),
		)

		startTime := time.Now()
		defer func() {
			duration := time.Since(startTime)
			if err != nil {
				p.logger.Error("ошибка обработки события",
					slog.String("event_type", eventType),
					slog.String("event_id", eventID),
					slog.String("handler_name", handlerName),
					slog.Any("error", err),
					slog.Duration("duration", duration),
				)
			} else {
				p.logger.Info("событие успешно обработано",
					slog.String("event_type", eventType),
					slog.String("event_id", eventID),
					slog.String("handler_name", handlerName),
					slog.Duration("duration", duration),
				)
			}
		}()

		return handler(ctx, event)
	}

	return p.next.Subscribe(wrappedHandler, opts...)
}

// Shutdown делегирует вызов следующему провайдеру в цепочке.
func (p *loggingProvider[T]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// metricsMiddleware реализует BusMiddleware для сбора метрик OpenTelemetry.
type metricsMiddleware[T Event] struct {
	meter               metric.Meter
	publishCounter      metric.Int64Counter
	consumeCounter      metric.Int64Counter
	consumeDurationHist metric.Float64Histogram
}

// NewMetricsMiddleware создает новое middleware для сбора метрик.
func NewMetricsMiddleware[T Event](provider metric.MeterProvider) BusMiddleware[T] {
	if provider == nil {
		return &noopMiddleware[T]{}
	}

	meter := provider.Meter(instrumentationName)

	publishCounter, err := meter.Int64Counter(
		metricKeyPrefix+"publish.count",
		metric.WithDescription("Количество опубликованных событий"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать счетчик publish.count: %v", err))
	}

	consumeCounter, err := meter.Int64Counter(
		metricKeyPrefix+"consume.count",
		metric.WithDescription("Количество обработанных событий"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать счетчик consume.count: %v", err))
	}

	consumeDurationHist, err := meter.Float64Histogram(
		metricKeyPrefix+"consume.duration",
		metric.WithDescription("Длительность обработки события"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать гистограмму consume.duration: %v", err))
	}

	return &metricsMiddleware[T]{
		meter:               meter,
		publishCounter:      publishCounter,
		consumeCounter:      consumeCounter,
		consumeDurationHist: consumeDurationHist,
	}
}

// Wrap оборачивает провайдер для добавления сбора метрик.
func (m *metricsMiddleware[T]) Wrap(next Provider[T]) Provider[T] {
	return &metricsProvider[T]{
		next:                next,
		publishCounter:      m.publishCounter,
		consumeCounter:      m.consumeCounter,
		consumeDurationHist: m.consumeDurationHist,
	}
}

// metricsProvider - это обертка над провайдером событий, которая собирает метрики.
type metricsProvider[T Event] struct {
	next                Provider[T]
	publishCounter      metric.Int64Counter
	consumeCounter      metric.Int64Counter
	consumeDurationHist metric.Float64Histogram
}

// Publish собирает метрики и публикует событие.
func (p *metricsProvider[T]) Publish(ctx context.Context, event T) (err error) {
	err = p.next.Publish(ctx, event)

	status := "success"
	if err != nil {
		status = "error"
	}
	eventType, _ := getEventTypeAndID(event)

	p.publishCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event.type", eventType),
		attribute.String("status", status),
	))

	return err
}

// Subscribe собирает метрики и подписывает обработчик.
func (p *metricsProvider[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	wrappedHandler := func(ctx context.Context, event T) (err error) {
		startTime := time.Now()

		err = handler(ctx, event)

		duration := float64(time.Since(startTime).Milliseconds())
		status := "success"
		if err != nil {
			status = "error"
		}

		eventType, _ := getEventTypeAndID(event)
		subOpts := &subscriptionOptions[T]{}
		for _, opt := range opts {
			opt(subOpts)
		}
		handlerName := subOpts.name
		if handlerName == "" {
			handlerName = getHandlerName(handler)
		}

		p.consumeCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("event.type", eventType),
			attribute.String("handler.name", handlerName),
			attribute.String("status", status),
		))

		p.consumeDurationHist.Record(ctx, duration, metric.WithAttributes(
			attribute.String("event.type", eventType),
			attribute.String("handler.name", handlerName),
			attribute.String("status", status),
		))

		return err
	}

	return p.next.Subscribe(wrappedHandler, opts...)
}

// Shutdown делегирует вызов следующему провайдеру.
func (p *metricsProvider[T]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// tracingMiddleware реализует BusMiddleware для распределенной трассировки OpenTelemetry.
type tracingMiddleware[T Event] struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewTracingMiddleware создает новое middleware для трассировки.
func NewTracingMiddleware[T Event](tp trace.TracerProvider, p propagation.TextMapPropagator) BusMiddleware[T] {
	if tp == nil {
		return &noopMiddleware[T]{}
	}

	if p == nil {
		p = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}

	return &tracingMiddleware[T]{
		tracer: tp.Tracer(
			instrumentationName,
			trace.WithInstrumentationVersion(instrumentationVersion),
		),
		propagator: p,
	}
}

// Wrap оборачивает провайдер для добавления логики трассировки.
func (m *tracingMiddleware[T]) Wrap(next Provider[T]) Provider[T] {
	return &tracingProvider[T]{
		next:       next,
		tracer:     m.tracer,
		propagator: m.propagator,
	}
}

// metadatable - это интерфейс для событий, которые могут переносить метаданные.
type metadatable interface {
	Metadata() map[string]string
}

// tracingProvider - это обертка над провайдером событий, которая управляет спанами трассировки.
type tracingProvider[T Event] struct {
	next       Provider[T]
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// Publish создает спан для публикации события и инъецирует контекст трассировки.
func (p *tracingProvider[T]) Publish(ctx context.Context, event T) (err error) {
	eventType, _ := getEventTypeAndID(event)
	spanName := fmt.Sprintf("%s publish", eventType)

	ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()

	if md, ok := (any(event)).(metadatable); ok {
		p.propagator.Inject(ctx, propagation.MapCarrier(md.Metadata()))
	}

	return p.next.Publish(ctx, event)
}

// Subscribe оборачивает обработчик для извлечения контекста трассировки и создания дочернего спана.
func (p *tracingProvider[T]) Subscribe(handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error) {
	wrappedHandler := func(ctx context.Context, event T) (err error) {
		if md, ok := (any(event)).(metadatable); ok {
			ctx = p.propagator.Extract(ctx, propagation.MapCarrier(md.Metadata()))
		}

		eventType, _ := getEventTypeAndID(event)
		spanName := fmt.Sprintf("%s process", eventType)

		ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
		defer func() {
			if err != nil {
				span.RecordError(err)
			}
			span.End()
		}()

		return handler(ctx, event)
	}

	return p.next.Subscribe(wrappedHandler, opts...)
}

// Shutdown делегирует вызов следующему провайдеру.
func (p *tracingProvider[T]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// applyMiddlewares применяет цепочку middleware к базовому провайдеру.
// Middleware применяются в обратном порядке, чтобы обеспечить правильную последовательность вызовов.
func applyMiddlewares[T Event](provider Provider[T], middlewares ...BusMiddleware[T]) Provider[T] {
	p := provider
	for i := len(middlewares) - 1; i >= 0; i-- {
		p = middlewares[i].Wrap(p)
	}
	return p
}

// noopMiddleware представляет собой пустое middleware, которое ничего не делает и просто вызывает следующий обработчик.
type noopMiddleware[T Event] struct{}

// Wrap просто возвращает следующий провайдер без изменений.
func (m *noopMiddleware[T]) Wrap(next Provider[T]) Provider[T] {
	return next
}

// getEventTypeAndID извлекает тип и ID события с помощью рефлексии.
func getEventTypeAndID(event any) (string, string) {
	val := reflect.ValueOf(event)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	eventType := val.Type().Name()
	eventID := "unknown"

	if idField := val.FieldByName("ID"); idField.IsValid() {
		eventID = fmt.Sprintf("%v", idField.Interface())
	}

	return eventType, eventID
}

// getHandlerName извлекает имя обработчика.
func getHandlerName(handler any) string {
	v := reflect.ValueOf(handler)
	if v.Kind() == reflect.Func {
		if pc := v.Pointer(); pc != 0 {
			if f := runtime.FuncForPC(pc); f != nil {
				return f.Name()
			}
		}
	}
	return reflect.TypeOf(handler).String()
}
