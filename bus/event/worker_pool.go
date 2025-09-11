package event

import (
	"sync"
)

// workerPool - это пул горутин для асинхронной обработки событий.
type workerPool[T Event] struct {
	minWorkers int
	maxWorkers int
	tasks      chan *Task[T]
	wg         sync.WaitGroup
	stopCh     chan struct{}
}

// newWorkerPool создает новый пул воркеров.
func newWorkerPool[T Event](min, max, queueSize int) *workerPool[T] {
	return &workerPool[T]{
		minWorkers: min,
		maxWorkers: max,
		tasks:      make(chan *Task[T], queueSize),
		stopCh:     make(chan struct{}),
	}
}

// run запускает воркеров пула.
func (p *workerPool[T]) run() {
	for i := 0; i < p.minWorkers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// stop останавливает всех воркеров и дожидается их завершения.
func (p *workerPool[T]) stop() {
	close(p.stopCh)
	p.wg.Wait()
}

// enqueue добавляет задачу в очередь на выполнение.
func (p *workerPool[T]) enqueue(task *Task[T]) {
	p.tasks <- task
}

// worker - это основная функция горутины-воркера.
func (p *workerPool[T]) worker() {
	defer p.wg.Done()
	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			// В реальном приложении здесь должна быть обработка ошибок из handler.
			_ = task.handler(task.ctx, task.event)
		case <-p.stopCh:
			return
		}
	}
}
