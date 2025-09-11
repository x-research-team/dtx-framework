package command

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"
)

// IDispatcher определяет основной, строго типизированный интерфейс для шины команд.
// Он отвечает за регистрацию обработчиков и диспетчеризацию команд.
type IDispatcher[C Command[R], R any] interface {
	// Dispatch отправляет команду в шину для выполнения.
	// Метод находит зарегистрированный обработчик для типа команды C,
	// выполняет его и возвращает результат типа R или ошибку.
	// Если обработчик для данной команды не найден, возвращается ошибка.
	Dispatch(ctx context.Context, cmd C) (R, error)

	// Register связывает тип команды C с ее обработчиком.
	// Попытка зарегистрировать обработчик для уже зарегистрированной команды
	// вернет ошибку.
	Register(handler CommandHandler[C, R]) error
}

// dispatcher представляет собой потокобезопасную реализацию IDispatcher.
// Он использует reflect.Type для сопоставления команд с их обработчиками,
// скрывая сложность дженериков за строго типизированным API.
type dispatcher[C Command[R], R any] struct {
	handler     CommandHandler[C, R]
	middlewares []Middleware[C, R]
	mu          sync.RWMutex
}

// NewDispatcher создает новый, готовый к использованию экземпляр диспетчера.
// Он принимает функциональные опции для конфигурации, например, для добавления middleware.
func NewDispatcher[C Command[R], R any](opts ...Option[C, R]) IDispatcher[C, R] {
	cfg := &config[C, R]{
		middlewares: make([]Middleware[C, R], 0),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return &dispatcher[C, R]{
		middlewares: cfg.middlewares,
	}
}

// Register регистрирует обработчик для конкретного типа команды.
// Этот метод является потокобезопасным.
// Возвращает ошибку, если обработчик для данной команды уже зарегистрирован.
func (d *dispatcher[C, R]) Register(handler CommandHandler[C, R]) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handler != nil {
		var cmd C
		cmdType := reflect.TypeOf(cmd)
		return fmt.Errorf("обработчик для команды '%s' уже зарегистрирован", cmdType)
	}

	// Оборачиваем базовый обработчик во все зарегистрированные middlewares.
	// Middlewares применяются в обратном порядке, чтобы обеспечить выполнение FIFO.
	h := handler
	for i := len(d.middlewares) - 1; i >= 0; i-- {
		h = d.middlewares[i](h)
	}
	d.handler = h

	return nil
}

// Dispatch находит и выполняет обработчик для указанной команды.
// Этот метод является потокобезопасным.
// Возвращает ошибку, если обработчик для данной команды не найден.
func (d *dispatcher[C, R]) Dispatch(ctx context.Context, cmd C) (R, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.handler == nil {
		var zero R
		cmdType := reflect.TypeOf(cmd)
		return zero, fmt.Errorf("обработчик для команды '%s' не найден", cmdType)
	}

	return d.handler(ctx, cmd)
}
