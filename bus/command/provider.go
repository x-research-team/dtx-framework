package command

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"
)

// Provider определяет контракт для сменных механизмов диспетчеризации команд.
type Provider[C Command[R], R any] interface {
	// Dispatch отправляет команду на выполнение.
	Dispatch(ctx context.Context, cmd C) (R, error)

	// Register регистрирует обработчик для команды.
	Register(handler CommandHandler[C, R]) error

	// Shutdown корректно завершает работу провайдера.
	Shutdown(ctx context.Context) error
}

// localProvider — это локальная, внутрипроцессная реализация провайдера команд.
type localProvider[C Command[R], R any] struct {
	handler CommandHandler[C, R]
	mu      sync.RWMutex
	cfg     *config[C, R]
}

// NewLocalProvider создает новый экземпляр локального провайдера.
func NewLocalProvider[C Command[R], R any](cfg *config[C, R]) (*localProvider[C, R], error) {
	return &localProvider[C, R]{
		cfg: cfg,
	}, nil
}

// Dispatch находит и выполняет обработчик для указанной команды.
func (p *localProvider[C, R]) Dispatch(ctx context.Context, cmd C) (R, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.handler == nil {
		var zero R
		cmdType := reflect.TypeOf(cmd)
		return zero, fmt.Errorf("обработчик для команды '%s' не найден", cmdType)
	}

	return p.handler(ctx, cmd)
}

// Register регистрирует обработчик для конкретного типа команды.
func (p *localProvider[C, R]) Register(handler CommandHandler[C, R]) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.handler != nil {
		var cmd C
		cmdType := reflect.TypeOf(cmd)
		return fmt.Errorf("обработчик для команды '%s' уже зарегистрирован", cmdType)
	}

	p.handler = handler
	return nil
}

// Shutdown в данной реализации не выполняет никаких действий.
func (p *localProvider[C, R]) Shutdown(ctx context.Context) error {
	return nil
}