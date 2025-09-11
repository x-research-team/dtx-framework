package event

import "context"

// Task представляет собой атомарную задачу для асинхронного выполнения:
// событие и соответствующий ему обработчик.
type Task[T Event] struct {
	ctx     context.Context
	event   T
	handler EventHandler[T]
}