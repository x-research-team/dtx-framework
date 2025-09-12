package command

import (
	"context"
	"fmt"
	"sync"

	"log/slog"
)

// IDispatcher определяет основной, строго типизированный интерфейс для шины команд.
type IDispatcher[C Command[R], R any] interface {
	Dispatch(ctx context.Context, cmd C) (R, error)
	Register(handler CommandHandler[C, R]) error
	Shutdown(ctx context.Context) error
}

// dispatcherImpl представляет собой реализацию IDispatcher.
type dispatcherImpl[C Command[R], R any] struct {
	provider Provider[C, R]
	cfg      *config[C, R]
	mu       sync.RWMutex
}

// NewDispatcher создает новый, готовый к использованию экземпляр диспетчера.
func NewDispatcher[C Command[R], R any](opts ...Option[C, R]) (IDispatcher[C, R], error) {
	cfg := &config[C, R]{
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	provider, err := NewLocalProvider[C, R](cfg)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать локальный провайдер: %w", err)
	}

	allMiddlewares := []Middleware[C, R]{
		NewLoggingMiddleware[C, R](cfg.logger),
		NewMetricsMiddleware[C, R](cfg.meterProvider),
		NewTracingMiddleware[C, R](cfg.tracerProvider, cfg.propagator),
	}
	allMiddlewares = append(allMiddlewares, cfg.middlewares...)
	wrappedProvider := applyMiddlewares(provider, allMiddlewares...)

	return &dispatcherImpl[C, R]{
		provider: wrappedProvider,
		cfg:      cfg,
	}, nil
}

// Register регистрирует обработчик для конкретного типа команды.
func (d *dispatcherImpl[C, R]) Register(handler CommandHandler[C, R]) error {
	return d.provider.Register(handler)
}

// Dispatch находит и выполняет обработчик для указанной команды.
func (d *dispatcherImpl[C, R]) Dispatch(ctx context.Context, cmd C) (R, error) {
	return d.provider.Dispatch(ctx, cmd)
}

// Shutdown корректно завершает работу диспетчера.
func (d *dispatcherImpl[C, R]) Shutdown(ctx context.Context) error {
	return d.provider.Shutdown(ctx)
}
