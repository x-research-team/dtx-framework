package event

import "context"

// Event - это интерфейс-маркер для всех событий.
// Он не несет методов и используется исключительно как ограничение (constraint)
// в обобщенных типах, гарантируя, что параметризующий тип является событием.
type Event any

// EventHandler - это строго типизированная функция-обработчик для события типа T.
type EventHandler[T Event] func(ctx context.Context, event T) error

// ErrorHandler - это строго типизированная функция для обработки ошибок,
// возникших в EventHandler для события типа T.
type ErrorHandler[T Event] func(err error, event T)

// Middleware - это строго типизированная функция-декоратор для EventHandler.
type Middleware[T Event] func(next EventHandler[T]) EventHandler[T]
