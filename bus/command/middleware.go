package command

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
	instrumentationName    = "github.com/x-research-team/dtx-framework/bus/command"
	instrumentationVersion = "0.1.0"
	metricKeyPrefix        = "messaging."
)

// Middleware определяет интерфейс для middleware шины команд.
type Middleware[C Command[R], R any] interface {
	Wrap(next Provider[C, R]) Provider[C, R]
}

// MiddlewareFunc является адаптером, позволяющим использовать обычные функции как middleware.
type MiddlewareFunc[C Command[R], R any] func(next Provider[C, R]) Provider[C, R]

// Wrap реализует интерфейс BusMiddleware.
func (f MiddlewareFunc[C, R]) Wrap(next Provider[C, R]) Provider[C, R] {
	return f(next)
}

// loggingMiddleware реализует BusMiddleware для логирования операций с командами.
type loggingMiddleware[C Command[R], R any] struct {
	logger *slog.Logger
}

// NewLoggingMiddleware создает новое middleware для логирования.
func NewLoggingMiddleware[C Command[R], R any](logger *slog.Logger) Middleware[C, R] {
	if logger == nil {
		return &noopMiddleware[C, R]{}
	}
	return &loggingMiddleware[C, R]{
		logger: logger,
	}
}

// Wrap оборачивает провайдер для добавления логирования.
func (m *loggingMiddleware[C, R]) Wrap(next Provider[C, R]) Provider[C, R] {
	return &loggingProvider[C, R]{
		next:   next,
		logger: m.logger,
	}
}

// loggingProvider - это обертка над провайдером команд, которая добавляет логирование.
type loggingProvider[C Command[R], R any] struct {
	next   Provider[C, R]
	logger *slog.Logger
}

// Dispatch логирует и отправляет команду.
func (p *loggingProvider[C, R]) Dispatch(ctx context.Context, cmd C) (result R, err error) {
	cmdType, cmdID := getCommandTypeAndID(cmd)
	p.logger.Info("отправка команды", slog.String("command_type", cmdType), slog.String("command_id", cmdID))

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if err != nil {
			p.logger.Error("ошибка отправки команды",
				slog.String("command_type", cmdType),
				slog.String("command_id", cmdID),
				slog.Any("error", err),
				slog.Duration("duration", duration),
			)
		}
	}()

	return p.next.Dispatch(ctx, cmd)
}

// Register логирует и регистрирует обработчик.
func (p *loggingProvider[C, R]) Register(handler CommandHandler[C, R]) (err error) {
	handlerName := getHandlerName(handler)
	p.logger.Info("регистрация обработчика команды", slog.String("handler_name", handlerName))
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
func (p *loggingProvider[C, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// metricsMiddleware реализует BusMiddleware для сбора метрик OpenTelemetry.
type metricsMiddleware[C Command[R], R any] struct {
	meter               metric.Meter
	dispatchCounter     metric.Int64Counter
	processDurationHist metric.Float64Histogram
}

// NewMetricsMiddleware создает новое middleware для сбора метрик.
func NewMetricsMiddleware[C Command[R], R any](provider metric.MeterProvider) Middleware[C, R] {
	if provider == nil {
		return &noopMiddleware[C, R]{}
	}

	meter := provider.Meter(instrumentationName)

	dispatchCounter, err := meter.Int64Counter(
		metricKeyPrefix+"dispatch.count",
		metric.WithDescription("Количество отправленных команд"),
		metric.WithUnit("{commands}"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать счетчик dispatch.count: %v", err))
	}

	processDurationHist, err := meter.Float64Histogram(
		metricKeyPrefix+"process.duration",
		metric.WithDescription("Длительность обработки команды"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic(fmt.Sprintf("не удалось создать гистограмму process.duration: %v", err))
	}

	return &metricsMiddleware[C, R]{
		meter:               meter,
		dispatchCounter:     dispatchCounter,
		processDurationHist: processDurationHist,
	}
}

// Wrap оборачивает провайдер для добавления сбора метрик.
func (m *metricsMiddleware[C, R]) Wrap(next Provider[C, R]) Provider[C, R] {
	return &metricsProvider[C, R]{
		next:                next,
		dispatchCounter:     m.dispatchCounter,
		processDurationHist: m.processDurationHist,
	}
}

// metricsProvider - это обертка над провайдером команд, которая собирает метрики.
type metricsProvider[C Command[R], R any] struct {
	next                Provider[C, R]
	dispatchCounter     metric.Int64Counter
	processDurationHist metric.Float64Histogram
}

// Dispatch собирает метрики и отправляет команду.
func (p *metricsProvider[C, R]) Dispatch(ctx context.Context, cmd C) (result R, err error) {
	startTime := time.Now()
	result, err = p.next.Dispatch(ctx, cmd)
	duration := float64(time.Since(startTime).Milliseconds())

	status := "success"
	if err != nil {
		status = "error"
	}
	cmdType, _ := getCommandTypeAndID(cmd)

	p.dispatchCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("command.type", cmdType),
		attribute.String("status", status),
	))

	p.processDurationHist.Record(ctx, duration, metric.WithAttributes(
		attribute.String("command.type", cmdType),
		attribute.String("status", status),
	))

	return result, err
}

// Register делегирует вызов.
func (p *metricsProvider[C, R]) Register(handler CommandHandler[C, R]) error {
	return p.next.Register(handler)
}

// Shutdown делегирует вызов.
func (p *metricsProvider[C, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// tracingMiddleware реализует BusMiddleware для распределенной трассировки OpenTelemetry.
type tracingMiddleware[C Command[R], R any] struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewTracingMiddleware создает новое middleware для трассировки.
func NewTracingMiddleware[C Command[R], R any](tp trace.TracerProvider, p propagation.TextMapPropagator) Middleware[C, R] {
	if tp == nil {
		return &noopMiddleware[C, R]{}
	}

	if p == nil {
		p = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}

	return &tracingMiddleware[C, R]{
		tracer: tp.Tracer(
			instrumentationName,
			trace.WithInstrumentationVersion(instrumentationVersion),
		),
		propagator: p,
	}
}

// Wrap оборачивает провайдер для добавления логики трассировки.
func (m *tracingMiddleware[C, R]) Wrap(next Provider[C, R]) Provider[C, R] {
	return &tracingProvider[C, R]{
		next:       next,
		tracer:     m.tracer,
		propagator: m.propagator,
	}
}

// tracingProvider - это обертка над провайдером команд, которая управляет спанами трассировки.
type tracingProvider[C Command[R], R any] struct {
	next       Provider[C, R]
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// Dispatch создает спан для отправки команды и инъецирует контекст трассировки.
func (p *tracingProvider[C, R]) Dispatch(ctx context.Context, cmd C) (result R, err error) {
	if md, ok := (any(cmd)).(Metadatable); ok {
		ctx = p.propagator.Extract(ctx, propagation.MapCarrier(md.Metadata()))
	}

	cmdType, _ := getCommandTypeAndID(cmd)
	spanName := fmt.Sprintf("%s process", cmdType)

	ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()

	return p.next.Dispatch(ctx, cmd)
}

// Register оборачивает обработчик для извлечения контекста трассировки и создания дочернего спана.
func (p *tracingProvider[C, R]) Register(handler CommandHandler[C, R]) error {
	wrappedHandler := func(ctx context.Context, cmd C) (R, error) {
		cmdType, _ := getCommandTypeAndID(cmd)
		spanName := fmt.Sprintf("%s publish", cmdType)

		ctx, span := p.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
		defer span.End()

		if md, ok := (any(cmd)).(Metadatable); ok {
			p.propagator.Inject(ctx, propagation.MapCarrier(md.Metadata()))
		}

		return handler(ctx, cmd)
	}
	return p.next.Register(wrappedHandler)
}

// Shutdown делегирует вызов.
func (p *tracingProvider[C, R]) Shutdown(ctx context.Context) error {
	return p.next.Shutdown(ctx)
}

// applyMiddlewares применяет цепочку middleware к базовому провайдеру.
func applyMiddlewares[C Command[R], R any](provider Provider[C, R], middlewares ...Middleware[C, R]) Provider[C, R] {
	p := provider
	for i := len(middlewares) - 1; i >= 0; i-- {
		p = middlewares[i].Wrap(p)
	}
	return p
}

// noopMiddleware представляет собой пустое middleware.
type noopMiddleware[C Command[R], R any] struct{}

// Wrap просто возвращает следующий провайдер без изменений.
func (m *noopMiddleware[C, R]) Wrap(next Provider[C, R]) Provider[C, R] {
	return next
}

// getCommandTypeAndID извлекает тип и ID команды с помощью рефлексии.
func getCommandTypeAndID(cmd any) (string, string) {
	val := reflect.ValueOf(cmd)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	cmdType := val.Type().Name()
	cmdID := "unknown"

	if idField := val.FieldByName("ID"); idField.IsValid() {
		cmdID = fmt.Sprintf("%v", idField.Interface())
	}

	return cmdType, cmdID
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
