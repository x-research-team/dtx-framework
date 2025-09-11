package event

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// config содержит неэкспортируемую конфигурацию для шины событий.
// Это позволяет добавлять новые опции без изменения публичного API.
type config[T Event] struct {
	logger         *slog.Logger
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	propagator     propagation.TextMapPropagator
	middlewares    []BusMiddleware[T]
}

// Option определяет тип для функциональных опций, которые изменяют конфигурацию шины.
type Option[T Event] func(*config[T])

// WithLogger возвращает опцию, которая устанавливает логгер для шины событий.
// Логгер используется для записи информации о жизненном цикле событий и ошибках.
func WithLogger[T Event](logger *slog.Logger) Option[T] {
	return func(c *config[T]) {
		c.logger = logger
	}
}

// WithTracerProvider возвращает опцию, которая устанавливает провайдер трассировки.
// Провайдер трассировки используется для создания и управления трассами в контексте OpenTelemetry.
func WithTracerProvider[T Event](provider trace.TracerProvider) Option[T] {
	return func(c *config[T]) {
		c.tracerProvider = provider
	}
}

// WithMeterProvider возвращает опцию, которая устанавливает провайдер метрик.
// Провайдер метрик используется для сбора и экспорта метрик производительности.
func WithMeterProvider[T Event](provider metric.MeterProvider) Option[T] {
	return func(c *config[T]) {
		c.meterProvider = provider
	}
}

// WithPropagator возвращает опцию, которая устанавливает механизм распространения контекста.
// Пропагатор отвечает за сериализацию и десериализацию контекста трассировки.
func WithPropagator[T Event](propagator propagation.TextMapPropagator) Option[T] {
	return func(c *config[T]) {
		c.propagator = propagator
	}
}

// WithBusMiddleware возвращает опцию, которая добавляет один или несколько middleware в цепочку обработки шины.
// Middleware выполняются в порядке их добавления.
func WithBusMiddleware[T Event](mw ...BusMiddleware[T]) Option[T] {
	return func(c *config[T]) {
		c.middlewares = append(c.middlewares, mw...)
	}
}