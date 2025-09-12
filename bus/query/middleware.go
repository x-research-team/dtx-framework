package query

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
	instrumentationName    = "github.com/x-research-team/dtx-framework/bus/query"
	instrumentationVersion = "0.1.0"
	metricKeyPrefix        = "messaging."
)

// Middleware определяет интерфейс для middleware шины запросов.
type Middleware[Q Query[R], R any] interface {
	Wrap(next Provider[Q, R]) Provider[Q, R]
}

// MiddlewareFunc является адаптером, позволяющим использовать обычные функции как middleware.
type MiddlewareFunc[Q Query[R], R any] func(next Provider[Q, R]) Provider[Q, R]

// Wrap реализует интерфейс BusMiddleware.
func (f MiddlewareFunc[Q, R]) Wrap(next Provider[Q, R]) Provider[Q, R] {
	return f(next)
}

// loggingMiddleware реализует BusMiddleware для логирования операций с запросами.
type loggingMiddleware[Q Query[R], R any] struct {
	logger *slog.Logger
}

// NewLoggingMiddleware создает новое middleware для логирования.
func NewLoggingMiddleware[Q Query[R], R any](logger *slog.Logger) Middleware[Q, R] {
	if logger == nil {
		return &noopMiddleware[Q, R]{}
	}
	return &loggingMiddleware[Q, R]{
		logger: logger,
	}
}

// Wrap оборачивает провайдер для добавления логирования.
func (m *loggingMiddleware[Q, R]) Wrap(next Provider[Q, R]) Provider[Q, R] {
	return &loggingProvider[Q, R]{
		next:   next,
		logger: m.logger,
	}
}

// loggingProvider - это обертка над провайдером запросов, которая добавляет логирование.
type loggingProvider[Q Query[R], R any] struct {
	next   Provider[Q, R]
	logger *slog.Logger
}

// Dispatch логирует и отправляет запрос.
func (p *loggingProvider[Q, R]) Dispatch(ctx context.Context, q Q) (result R, err error) {
	queryType, queryID := getQueryTypeAndID(q)
	p.logger.Info("отправка запроса", slog.String("query_type", queryType), slog.String("query_id", queryID))

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if err != nil {
			p.logger.Error("ошибка отправки запроса",
				slog.String("query_type", queryType),
				slog.String("query_id", queryID),
				slog.Any("error", err),
				slog.Duration("duration", duration),
			)
		}
	}()

	return p.next.Dispatch(ctx, q)
}

// Register логирует и регистрирует обработчик.
func (p *loggingProvider[Q, R]) Register(handler QueryHandler[Q, R]) (err error) {
	handlerName := getHandlerName(handler)
	p.logger.Info("регистрация обработчика запроса", slog.String("handler_name", handlerName))
	defer func() {
		if err != nil {
			p.logger.Error("ошибка регистрации обработчика",
				slog.String("handler_name", handlerName),
				slog.Any("error", err),
			)
		}
	}()
	return p.next.Register(handler)
}

// Shutdown делегирует вызов следующему провайдеру в цепочке.
func (p *loggingProvider[Q, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// metricsMiddleware реализует BusMiddleware для сбора метрик OpenTelemetry.
type metricsMiddleware[Q Query[R], R any] struct {
	meter               metric.Meter
	dispatchCounter     metric.Int64Counter
	processDurationHist metric.Float64Histogram
}

// NewMetricsMiddleware создает новое middleware для сбора метрик.
func NewMetricsMiddleware[Q Query[R], R any](provider metric.MeterProvider) Middleware[Q, R] {
	if provider == nil {
		return &noopMiddleware[Q, R]{}
	}

	meter := provider.Meter(instrumentationName)

	dispatchCounter, err := meter.Int64Counter(
		metricKeyPrefix+"dispatch.count",
		metric.WithDescription("Количество отправленных запросов"),
		metric.WithUnit("{queries}"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать счетчик dispatch.count: %v", err))
	}

	processDurationHist, err := meter.Float64Histogram(
		metricKeyPrefix+"process.duration",
		metric.WithDescription("Длительность обработки запроса"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать гистограмму process.duration: %v", err))
	}

	return &metricsMiddleware[Q, R]{
		meter:               meter,
		dispatchCounter:     dispatchCounter,
		processDurationHist: processDurationHist,
	}
}

// Wrap оборачивает провайдер для добавления сбора метрик.
func (m *metricsMiddleware[Q, R]) Wrap(next Provider[Q, R]) Provider[Q, R] {
	return &metricsProvider[Q, R]{
		next:                next,
		dispatchCounter:     m.dispatchCounter,
		processDurationHist: m.processDurationHist,
	}
}

// metricsProvider - это обертка над провайдером запросов, которая собирает метрики.
type metricsProvider[Q Query[R], R any] struct {
	next                Provider[Q, R]
	dispatchCounter     metric.Int64Counter
	processDurationHist metric.Float64Histogram
}

// Dispatch собирает метрики и отправляет запрос.
func (p *metricsProvider[Q, R]) Dispatch(ctx context.Context, q Q) (result R, err error) {
	startTime := time.Now()
	result, err = p.next.Dispatch(ctx, q)
	duration := float64(time.Since(startTime).Milliseconds())

	status := "success"
	if err != nil {
		status = "error"
	}
	queryType, _ := getQueryTypeAndID(q)

	p.dispatchCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("query.type", queryType),
		attribute.String("status", status),
	))

	p.processDurationHist.Record(ctx, duration, metric.WithAttributes(
		attribute.String("query.type", queryType),
		attribute.String("status", status),
	))

	return result, err
}

// Register делегирует вызов.
func (p *metricsProvider[Q, R]) Register(handler QueryHandler[Q, R]) error {
	return p.next.Register(handler)
}

// Shutdown делегирует вызов.
func (p *metricsProvider[Q, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// tracingMiddleware реализует BusMiddleware для распределенной трассировки OpenTelemetry.
type tracingMiddleware[Q Query[R], R any] struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewTracingMiddleware создает новое middleware для трассировки.
func NewTracingMiddleware[Q Query[R], R any](tp trace.TracerProvider, p propagation.TextMapPropagator) Middleware[Q, R] {
	if tp == nil {
		return &noopMiddleware[Q, R]{}
	}

	if p == nil {
		p = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}

	return &tracingMiddleware[Q, R]{
		tracer: tp.Tracer(
			instrumentationName,
			trace.WithInstrumentationVersion(instrumentationVersion),
		),
		propagator: p,
	}
}

// Wrap оборачивает провайдер для добавления логики трассировки.
func (m *tracingMiddleware[Q, R]) Wrap(next Provider[Q, R]) Provider[Q, R] {
	return &tracingProvider[Q, R]{
		next:       next,
		tracer:     m.tracer,
		propagator: m.propagator,
	}
}

// tracingProvider - это обертка над провайдером запросов, которая управляет спанами трассировки.
type tracingProvider[Q Query[R], R any] struct {
	next       Provider[Q, R]
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// Dispatch создает спан для отправки запроса и инъецирует контекст трассировки.
func (p *tracingProvider[Q, R]) Dispatch(ctx context.Context, q Q) (result R, err error) {
	if md, ok := (any(q)).(Metadatable); ok {
		ctx = p.propagator.Extract(ctx, propagation.MapCarrier(md.Metadata()))
	}

	queryType, _ := getQueryTypeAndID(q)
	spanName := fmt.Sprintf("%s process", queryType)

	ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()

	return p.next.Dispatch(ctx, q)
}

// Register оборачивает обработчик для извлечения контекста трассировки и создания дочернего спана.
func (p *tracingProvider[Q, R]) Register(handler QueryHandler[Q, R]) error {
	wrappedHandler := func(ctx context.Context, q Q) (R, error) {
		queryType, _ := getQueryTypeAndID(q)
		spanName := fmt.Sprintf("%s publish", queryType)

		ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
		defer span.End()

		if md, ok := (any(q)).(Metadatable); ok {
			p.propagator.Inject(ctx, propagation.MapCarrier(md.Metadata()))
		}

		return handler(ctx, q)
	}
	return p.next.Register(wrappedHandler)
}

// Shutdown делегирует вызов.
func (p *tracingProvider[Q, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// applyMiddlewares применяет цепочку middleware к базовому провайдеру.
func applyMiddlewares[Q Query[R], R any](provider Provider[Q, R], middlewares ...Middleware[Q, R]) Provider[Q, R] {
	p := provider
	for i := len(middlewares) - 1; i >= 0; i-- {
		p = middlewares[i].Wrap(p)
	}
	return p
}

// noopMiddleware представляет собой пустое middleware.
type noopMiddleware[Q Query[R], R any] struct{}

// Wrap просто возвращает следующий провайдер без изменений.
func (m *noopMiddleware[Q, R]) Wrap(next Provider[Q, R]) Provider[Q, R] {
	return next
}

// getQueryTypeAndID извлекает тип и ID запроса с помощью рефлексии.
func getQueryTypeAndID(q any) (string, string) {
	val := reflect.ValueOf(q)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	queryType := val.Type().Name()
	queryID := "unknown"

	if idField := val.FieldByName("ID"); idField.IsValid() {
		queryID = fmt.Sprintf("%v", idField.Interface())
	}

	return queryType, queryID
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
