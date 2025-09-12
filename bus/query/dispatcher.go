package query

import (
	"context"
	"fmt"
	"sync"

	"log/slog"
)

// IDispatcher определяет основной, строго типизированный интерфейс для шины запросов.
type IDispatcher[Q Query[R], R any] interface {
	Dispatch(ctx context.Context, q Q) (R, error)
	Register(handler QueryHandler[Q, R]) error
	Shutdown(ctx context.Context) error
}

// dispatcherImpl представляет собой реализацию IDispatcher.
type dispatcherImpl[Q Query[R], R any] struct {
	provider Provider[Q, R]
	cfg      *config[Q, R]
	mu       sync.RWMutex
}

// NewDispatcher создает новый, готовый к использованию экземпляр диспетчера.
func NewDispatcher[Q Query[R], R any](opts ...Option[Q, R]) (IDispatcher[Q, R], error) {
	cfg := &config[Q, R]{
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	provider, err := NewLocalProvider[Q, R](cfg)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать локальный провайдер: %w", err)
	}

	allMiddlewares := []Middleware[Q, R]{
		NewLoggingMiddleware[Q, R](cfg.logger),
		NewMetricsMiddleware[Q, R](cfg.meterProvider),
		NewTracingMiddleware[Q, R](cfg.tracerProvider, cfg.propagator),
	}
	allMiddlewares = append(allMiddlewares, cfg.middlewares...)
	wrappedProvider := applyMiddlewares(provider, allMiddlewares...)

	return &dispatcherImpl[Q, R]{
		provider: wrappedProvider,
		cfg:      cfg,
	}, nil
}

// Register регистрирует обработчик для конкретного типа запроса.
func (d *dispatcherImpl[Q, R]) Register(handler QueryHandler[Q, R]) error {
	return d.provider.Register(handler)
}

// Dispatch находит и выполняет обработчик для указанного запроса.
func (d *dispatcherImpl[Q, R]) Dispatch(ctx context.Context, q Q) (R, error) {
	return d.provider.Dispatch(ctx, q)
}

// Shutdown корректно завершает работу диспетчера.
func (d *dispatcherImpl[Q, R]) Shutdown(ctx context.Context) error {
	return d.provider.Shutdown(ctx)
}
