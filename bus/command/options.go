package command

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// config содержит неэкспортируемую конфигурацию для шины команд.
type config[C Command[R], R any] struct {
	logger         *slog.Logger
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	propagator     propagation.TextMapPropagator
	middlewares    []Middleware[C, R]
}

// Option определяет тип для функциональных опций, которые изменяют конфигурацию шины.
type Option[C Command[R], R any] func(*config[C, R])

// WithLogger возвращает опцию, которая устанавливает логгер для шины.
func WithLogger[C Command[R], R any](logger *slog.Logger) Option[C, R] {
	return func(c *config[C, R]) {
		c.logger = logger
	}
}

// WithTracerProvider возвращает опцию, которая устанавливает провайдер трассировки.
func WithTracerProvider[C Command[R], R any](provider trace.TracerProvider) Option[C, R] {
	return func(c *config[C, R]) {
		c.tracerProvider = provider
	}
}

// WithMeterProvider возвращает опцию, которая устанавливает провайдер метрик.
func WithMeterProvider[C Command[R], R any](provider metric.MeterProvider) Option[C, R] {
	return func(c *config[C, R]) {
		c.meterProvider = provider
	}
}

// WithPropagator возвращает опцию, которая устанавливает механизм распространения контекста.
func WithPropagator[C Command[R], R any](propagator propagation.TextMapPropagator) Option[C, R] {
	return func(c *config[C, R]) {
		c.propagator = propagator
	}
}

// WithMiddleware возвращает опцию, которая добавляет один или несколько middleware в цепочку обработки.
func WithMiddleware[C Command[R], R any](mw ...Middleware[C, R]) Option[C, R] {
	return func(c *config[C, R]) {
		c.middlewares = append(c.middlewares, mw...)
	}
}
