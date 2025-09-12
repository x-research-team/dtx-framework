package query

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"
)

// Provider определяет контракт для сменных механизмов диспетчеризации запросов.
type Provider[Q Query[R], R any] interface {
	// Dispatch отправляет запрос на выполнение.
	Dispatch(ctx context.Context, q Q) (R, error)

	// Register регистрирует обработчик для запроса.
	Register(handler QueryHandler[Q, R]) error

	// Shutdown корректно завершает работу провайдера.
	Shutdown(ctx context.Context) error
}

// localProvider — это локальная, внутрипроцессная реализация провайдера запросов.
type localProvider[Q Query[R], R any] struct {
	handler QueryHandler[Q, R]
	mu      sync.RWMutex
	cfg     *config[Q, R]
}

// NewLocalProvider создает новый экземпляр локального провайдера.
func NewLocalProvider[Q Query[R], R any](cfg *config[Q, R]) (*localProvider[Q, R], error) {
	return &localProvider[Q, R]{
		cfg: cfg,
	}, nil
}

// Dispatch находит и выполняет обработчик для указанного запроса.
func (p *localProvider[Q, R]) Dispatch(ctx context.Context, q Q) (R, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.handler == nil {
		var zero R
		queryType := reflect.TypeOf(q)
		return zero, fmt.Errorf("обработчик для запроса '%s' не найден", queryType)
	}

	return p.handler(ctx, q)
}

// Register регистрирует обработчик для конкретного типа запроса.
func (p *localProvider[Q, R]) Register(handler QueryHandler[Q, R]) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.handler != nil {
		var q Q
		queryType := reflect.TypeOf(q)
		return fmt.Errorf("обработчик для запроса '%s' уже зарегистрирован", queryType)
	}

	p.handler = handler
	return nil
}

// Shutdown в данной реализации не выполняет никаких действий.
func (p *localProvider[Q, R]) Shutdown(ctx context.Context) error {
	return nil
}