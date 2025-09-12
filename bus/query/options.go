package query

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// config содержит неэкспортируемую конфигурацию для шины запросов.
type config[Q Query[R], R any] struct {
	logger         *slog.Logger
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	propagator     propagation.TextMapPropagator
	middlewares    []Middleware[Q, R]
}

// Option определяет тип для функциональных опций, которые изменяют конфигурацию шины.
type Option[Q Query[R], R any] func(*config[Q, R])

// WithLogger возвращает опцию, которая устанавливает логгер для шины.
func WithLogger[Q Query[R], R any](logger *slog.Logger) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.logger = logger
	}
}

// WithTracerProvider возвращает опцию, которая устанавливает провайдер трассировки.
func WithTracerProvider[Q Query[R], R any](provider trace.TracerProvider) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.tracerProvider = provider
	}
}

// WithMeterProvider возвращает опцию, которая устанавливает провайдер метрик.
func WithMeterProvider[Q Query[R], R any](provider metric.MeterProvider) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.meterProvider = provider
	}
}

// WithPropagator возвращает опцию, которая устанавливает механизм распространения контекста.
func WithPropagator[Q Query[R], R any](propagator propagation.TextMapPropagator) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.propagator = propagator
	}
}

// WithMiddleware возвращает опцию, которая добавляет один или несколько middleware в цепочку обработки.
func WithMiddleware[Q Query[R], R any](mw ...Middleware[Q, R]) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.middlewares = append(c.middlewares, mw...)
	}
}
