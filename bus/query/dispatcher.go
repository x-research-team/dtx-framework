package query

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"
)

// IDispatcher определяет основной, строго типизированный интерфейс для шины запросов.
// Он отвечает за регистрацию обработчиков и диспетчеризацию запросов.
type IDispatcher[Q Query[R], R any] interface {
	// Dispatch отправляет запрос в шину для выполнения.
	// Метод находит зарегистрированный обработчик для типа запроса Q,
	// выполняет его и возвращает результат типа R или ошибку.
	// Если обработчик для данного запроса не найден, возвращается ошибка.
	Dispatch(ctx context.Context, q Q) (R, error)

	// Register связывает тип запроса Q с его обработчиком.
	// Попытка зарегистрировать обработчик для уже зарегистрированного запроса
	// вернет ошибку.
	Register(handler QueryHandler[Q, R]) error
}

// dispatcher представляет собой потокобезопасную реализацию IDispatcher.
// Он использует reflect.Type для сопоставления запросов с их обработчиками,
// скрывая сложность дженериков за строго типизированным API.
type dispatcher[Q Query[R], R any] struct {
	handler     QueryHandler[Q, R]
	middlewares []Middleware[Q, R]
	mu          sync.RWMutex
}

// NewDispatcher создает новый, готовый к использованию экземпляр диспетчера.
// Он принимает функциональные опции для конфигурации, например, для добавления middleware.
func NewDispatcher[Q Query[R], R any](opts ...Option[Q, R]) IDispatcher[Q, R] {
	cfg := &config[Q, R]{
		middlewares: make([]Middleware[Q, R], 0),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return &dispatcher[Q, R]{
		middlewares: cfg.middlewares,
	}
}

// Register регистрирует обработчик для конкретного типа запроса.
// Этот метод является потокобезопасным.
// Возвращает ошибку, если обработчик для данного запроса уже зарегистрирован.
func (d *dispatcher[Q, R]) Register(handler QueryHandler[Q, R]) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handler != nil {
		var q Q
		qType := reflect.TypeOf(q)
		return fmt.Errorf("обработчик для запроса '%s' уже зарегистрирован", qType)
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

// Dispatch находит и выполняет обработчик для указанного запроса.
// Этот метод является потокобезопасным.
// Возвращает ошибку, если обработчик для данного запроса не найден.
func (d *dispatcher[Q, R]) Dispatch(ctx context.Context, q Q) (R, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.handler == nil {
		var zero R
		qType := reflect.TypeOf(q)
		return zero, fmt.Errorf("обработчик для запроса '%s' не найден", qType)
	}

	return d.handler(ctx, q)
}

