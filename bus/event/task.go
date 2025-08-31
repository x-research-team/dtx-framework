package event

import (
	"context"
	"sync"
)

// task представляет собой задачу для обработки в пуле воркеров.
type task struct {
	ctx          context.Context
	event        Event
	subscription *subscription
}

// reset сбрасывает состояние задачи, чтобы её можно было безопасно переиспользовать.
func (t *task) reset() {
	t.ctx = nil
	t.event = nil
	t.subscription = nil
}

// taskPool — это пул для переиспользования объектов task, чтобы избежать лишних аллокаций.
var taskPool = sync.Pool{
	New: func() any {
		return new(task)
	},
}