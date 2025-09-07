package event

// Task представляет собой атомарную задачу для асинхронного выполнения:
// событие и соответствующий ему обработчик.
type Task[T Event] struct {
	event   T
	handler EventHandler[T]
}