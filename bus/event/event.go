package event

import "context"

// Event определяет интерфейс для всех событий в системе.
// Любое событие должно предоставлять метаданные для поддержки сквозных функций,
// таких как трассировка и передача контекста.
type Event interface {
	// Metadata возвращает карту метаданных события.
	// Эта карта используется для передачи технической информации (например, ID трассировки)
	// между сервисами и компонентами.
	Metadata() map[string]string
}

// EventHandler - это строго типизированная функция-обработчик для события типа T.
type EventHandler[T Event] func(ctx context.Context, event T) error

// ErrorHandler - это строго типизированная функция для обработки ошибок,
// возникших в EventHandler для события типа T.
type ErrorHandler[T Event] func(err error, event T)

// Middleware - это строго типизированная функция-декоратор для EventHandler.
type Middleware[T Event] func(next EventHandler[T]) EventHandler[T]
