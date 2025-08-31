package event

import (
	"context"
	"sync"
)

// workerPool управляет пулом горутин для асинхронной обработки событий.
// Он обеспечивает ограничение на количество одновременно выполняющихся задач
// и корректное завершение работы (graceful shutdown).
type workerPool struct {
	// minWorkers задает минимальное количество горутин, которые всегда
	// активны и готовы к обработке задач.
	minWorkers int
	// maxWorkers задает максимальное количество горутин, которое может
	// быть запущено в пуле. На данный момент автомасштабирование не реализовано,
	// и используется `minWorkers`.
	maxWorkers int
	// tasks — это буферизированный канал, через который воркеры получают
	// задачи для выполнения. Размер канала определяет, сколько задач
	// может ожидать в очереди.
	tasks chan *task
	// wg — это WaitGroup для синхронизации и ожидания завершения всех
	// горутин-воркеров при остановке пула.
	wg sync.WaitGroup
	// cancelFunc — функция для отмены корневого контекста пула,
	// что служит сигналом для всех воркеров о необходимости завершения работы.
	cancelFunc context.CancelFunc
	// logger — экземпляр логгера для записи информации о работе пула,
	// включая ошибки и паники в обработчиках.
	logger Logger
}

// newWorkerPool создает и инициализирует новый пул воркеров.
func newWorkerPool(min, max, queueSize int, logger Logger) *workerPool {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &workerPool{
		minWorkers: min,
		maxWorkers: max,
		tasks:      make(chan *task, queueSize),
		logger:     logger,
	}
}

// start запускает воркеров пула.
// Создается корневой контекст для управления жизненным циклом всех воркеров.
func (p *workerPool) start() {
	var ctx context.Context
	ctx, p.cancelFunc = context.WithCancel(context.Background())

	for i := 0; i < p.minWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// stop инициирует корректное завершение работы пула воркеров.
// Процесс состоит из трех шагов:
//  1. Закрытие канала `tasks`, чтобы новые задачи больше не принимались.
//  2. Отмена контекста, что является сигналом для всех активных воркеров
//     завершить свою работу после выполнения текущей задачи.
//  3. Ожидание завершения всех горутин-воркеров с помощью `wg.Wait()`.
//
// Этот механизм гарантирует, что все задачи, уже находящиеся в очереди,
// будут обработаны до полной остановки пула.
func (p *workerPool) stop() {
	// Шаг 1: Прекращаем прием новых задач.
	// Паника при отправке в закрытый канал будет обработана в `submit`.
	close(p.tasks)

	// Шаг 2: Сигнализируем всем воркерам о необходимости завершения.
	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	// Шаг 3: Ожидаем, пока все воркеры полностью завершат свою работу.
	p.wg.Wait()
}

// submit безопасно отправляет задачу в пул для асинхронной обработки.
// Возвращает false, если пул уже остановлен и не может принять задачу.
func (p *workerPool) submit(ctx context.Context, event Event, sub *subscription) (ok bool) {
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.event = event
	t.subscription = sub

	defer func() {
		if r := recover(); r != nil {
			// Паника может произойти, если канал tasks уже закрыт.
			// Это ожидаемое поведение при остановке.
			p.logger.Warnf("Не удалось отправить задачу в пул: %v", r)
			ok = false
			// Возвращаем задачу в пул, так как она не была обработана.
			t.reset()
			taskPool.Put(t)
		}
	}()

	p.tasks <- t
	return true
}

// ProcessSync выполняет задачу синхронно, используя тот же механизм обработки,
// что и воркеры, но без отправки в канал.
// Это позволяет переиспользовать логику обработки и восстановления после паник.
func (p *workerPool) ProcessSync(ctx context.Context, event Event, sub *subscription) {
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.event = event
	t.subscription = sub

	p.processTask(t)
}

// worker — это основная функция, выполняемая каждой горутиной в пуле.
// Она бесконечно ожидает задачи из канала и обрабатывает их.
// Воркер завершает работу, когда контекст отменяется или канал задач закрывается.
func (p *workerPool) worker(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-p.tasks:
			if !ok {
				return
			}
			p.processTask(t)
		}
	}
}

// processTask выполняет одну задачу.
// Важно, что здесь реализован механизм recover для перехвата паник
// внутри обработчика событий, чтобы одна паника не обрушила весь воркер.
// После выполнения задача очищается и возвращается в пул.
func (p *workerPool) processTask(t *task) {
	// Гарантируем, что задача будет очищена и возвращена в пул после выполнения.
	defer func() {
		if r := recover(); r != nil {
			p.logger.Errorf(
				"Паника в обработчике события. Топик: %s, Событие: %+v, Ошибка: %v",
				t.event.Topic(),
				t.event,
				r,
			)
		}
		t.reset()
		taskPool.Put(t)
	}()

	// Здесь будет вызвана обертка, которая выполнит приведение типа
	// и вызовет типизированный обработчик.
	handler, ok := t.subscription.handler.(EventHandler[Event])
	if !ok {
		p.logger.Errorf("Не удалось привести обработчик к ожидаемому типу EventHandler[Event] для топика %s", t.event.Topic())
		return
	}

	if err := handler(t.ctx, t.event); err != nil {
		// Если у подписки есть свой обработчик ошибок, используем его.
		// Приведение типа к ErrorHandler[Event] обеспечивает типобезопасность.
		if t.subscription.errorHandler != nil {
			if eh, ok := t.subscription.errorHandler.(ErrorHandler[Event]); ok {
				eh(err, t.event)
				return // После кастомной обработки, стандартную не выполняем.
			}
		}

		// Стандартное логирование ошибки, если кастомный обработчик не задан.
		p.logger.Errorf(
			"Ошибка при обработке события. Топик: %s, Событие: %+v, Ошибка: %v",
			t.event.Topic(),
			t.event,
			err,
		)
	}
}
