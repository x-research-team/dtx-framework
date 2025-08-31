package event

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"

	"github.com/google/uuid"
)

// Bus — это центральный компонент, реализующий EventBus.
// Он управляет подписчиками, обрабатывает публикацию событий и координирует
// асинхронное выполнение через внутренний пул воркеров.
// Структура является потокобезопасной.
type Bus[T Event] struct {
	// subscribers хранит карту подписок, где ключ — это топик,
	// а значение — срез активных подписок на этот топик.
	subscribers map[string][]*subscription
	// mu — это RWMutex для обеспечения потокобезопасного доступа
	// к карте `subscribers`. Использование RWMutex оптимизирует
	// производительность для сценариев с частыми чтениями (Publish)
	// и редкими записями (Subscribe/Unsubscribe).
	mu sync.RWMutex
	// cache — это высокопроизводительный, потокобезопасный кеш для срезов
	// подписчиков. Он использует `sync.Map` для минимизации блокировок
	// на горячем пути (Publish). Ключ — топик, значение — `[]*subscription`.
	// Кеш инвалидируется при каждом изменении подписок (Subscribe/Unsubscribe).
	cache sync.Map
	// opts хранит конфигурационные параметры шины,
	// установленные при ее создании.
	opts busOptions
	// pool — это внутренний пул воркеров для обработки
	// асинхронных событий.
	pool *workerPool
	// wg используется для ожидания завершения всех активных
	// горутин-обработчиков во время корректного завершения работы (Shutdown).
	wg sync.WaitGroup
	// dispatchWg используется для синхронизации и ожидания завершения
	// всех операций диспетчеризации перед остановкой пула воркеров.
	// Это предотвращает состояние гонки, когда Shutdown может начаться
	// до того, как все события будут отправлены в пул.
	dispatchWg sync.WaitGroup
	// slicePool — это пул для переиспользования срезов подписчиков,
	// чтобы избежать аллокаций на горячем пути в методе Publish.
	slicePool sync.Pool
}

// subscription представляет собой внутреннюю структуру для хранения информации
// о конкретной подписке. Она содержит все необходимые данные для вызова
// обработчика, включая его самого и примененные к нему опции.
type subscription struct {
	// id представляет собой уникальный идентификатор подписки (UUID),
	// который используется для ее безопасного удаления (отписки).
	id string
	// topic — топик, на который оформлена данная подписка.
	topic string
	// handler — это функция-обработчик события. Хранится как `any` для
	// универсальности, но приводится к конкретному типу `EventHandler[T]`
	// перед вызовом для обеспечения типобезопасности.
	handler any
	// isAsync — флаг, указывающий, должна ли обработка события выполняться
	// асинхронно в отдельной горутине через пул воркеров.
	isAsync bool
	// errorHandler — это опциональная функция для пользовательской обработки
	// ошибок, возникающих во время выполнения `handler`.
	errorHandler any
}

// NewBus создает новый экземпляр Bus.
// Принимает функциональные опции для гибкой настройки.
// Если логгер или сборщик метрик не предоставлены, используются
// реализации-заглушки (noop), чтобы избежать паники при nil-вызове.
func NewBus(opts ...BusOption) *Bus[Event] {
	options := busOptions{
		// Устанавливаем значения по умолчанию
		workerMin: 1,
		workerMax: 10,
		queueSize: 100,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if options.logger == nil {
		options.logger = &noopLogger{}
	}
	if options.metrics == nil {
		options.metrics = &noopMetrics{}
	}

	pool := newWorkerPool(options.workerMin, options.workerMax, options.queueSize, options.logger)
	pool.start()

	b := &Bus[Event]{
		subscribers: make(map[string][]*subscription),
		opts:        options,
		pool:        pool,
	}
	b.slicePool.New = func() any {
		// Создаем срез с capacity, чтобы избежать лишних аллокаций при append.
		// Начальный размер 0, но с запасом.
		s := make([]*subscription, 0, 10)
		return &s
	}
	return b
}

// Publish публикует событие в шину.
// Метод находит всех подписчиков на топик события и, на данном этапе,
// просто завершается. Логика диспетчеризации будет добавлена позже.
// Используется RLock для обеспечения высокой производительности при частых чтениях.
func (b *Bus[T]) Publish(ctx context.Context, event Event) error {
	topic := event.Topic()
	var subsToProcess []*subscription

	// Этап 1: Попытка чтения из кеша (горячий путь, без блокировок).
	if cachedSubs, ok := b.cache.Load(topic); ok {
		// Если в кеше есть запись, используем ее.
		// Приведение типа необходимо, так как sync.Map хранит any.
		subsToProcess = cachedSubs.([]*subscription)
	} else {
		// Этап 2: Кеш-промах. Переходим к медленному пути с блокировкой.
		b.mu.RLock()
		// Получаем срез из пула для создания копии.
		subsSlicePtr := b.slicePool.Get().(*[]*subscription)
		// Копируем подписчиков, чтобы избежать гонки данных после разблокировки.
		if subs, ok := b.subscribers[topic]; ok {
			*subsSlicePtr = append(*subsSlicePtr, subs...)
		}
		b.mu.RUnlock()

		// Сохраняем копию в кеш для будущих вызовов.
		// Это атомарная операция, которая не блокирует другие чтения.
		subsToProcess = *subsSlicePtr
		b.cache.Store(topic, subsToProcess)

		// Важно: не возвращаем срез в пул, так как он теперь хранится в кеше.
		// Вместо этого, при инвалидации кеша, мы должны будем вернуть его.
		// (Эта логика будет добавлена в Subscribe/Unsubscribe).
		// На данном этапе для простоты мы просто не возвращаем его.
		// TODO: Реализовать возврат среза в пул при инвалидации кеша.
	}

	// Диспетчеризация событий для подписчиков.
	for _, sub := range subsToProcess {
		b.dispatch(ctx, event, sub)
	}

	return nil
}

// Subscribe подписывает обработчик на события определенного топика.
// Метод является потокобезопасным. Он создает уникальную подписку,
// сохраняет ее и возвращает функцию для отписки.
func (b *Bus[T]) Subscribe(topic string, handler any, opts ...SubscribeOption[Event]) (unsubscribe func(), err error) {
	// Создаем обертку с использованием рефлексии.
	wrappedHandler, err := wrapHandler(handler)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания обертки обработчика: %w", err)
	}

	subOpts := subscriptionOptions[Event]{}
	for _, opt := range opts {
		opt(&subOpts)
	}

	// Применяем middleware к уже созданной универсальной обертке.
	finalHandler := wrappedHandler
	for i := len(subOpts.middleware) - 1; i >= 0; i-- {
		finalHandler = subOpts.middleware[i](finalHandler)
	}

	sub := &subscription{
		id:           uuid.NewString(),
		topic:        topic,
		handler:      finalHandler, // Сохраняем финальный обработчик
		isAsync:      subOpts.isAsync,
		errorHandler: subOpts.errorHandler,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers[topic] = append(b.subscribers[topic], sub)
	// Инвалидируем кеш для данного топика, так как список подписчиков изменился.
	b.cache.Delete(topic)

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subs := b.subscribers[topic]
		for i, s := range subs {
			if s.id == sub.id {
				b.subscribers[topic] = append(subs[:i], subs[i+1:]...)
				// Инвалидируем кеш после отписки.
				b.cache.Delete(topic)
				break
			}
		}
	}, nil
}

// Shutdown инициирует корректное завершение работы шины.
// Этот процесс включает два ключевых этапа:
//  1. Ожидание завершения всех горутин-диспетчеров, которые были запущены
//     для обработки входящих событий. Это гарантирует, что все события,
//     принятые на момент вызова Shutdown, будут переданы в пул воркеров
//     или выполнены синхронно.
//  2. Остановка пула воркеров, которая, в свою очередь, обеспечивает
//     обработку всех задач, находящихся в очереди.
//
// Таким образом, гарантируется полная обработка всех событий до выключения.
func (b *Bus[T]) Shutdown(ctx context.Context) error {
	// Шаг 1: Дожидаемся завершения всех горутин-диспетчеров.
	// Это гарантирует, что все вызовы Publish, сделанные до Shutdown,
	// успели запустить свои горутины для обработки.
	b.wg.Wait()

	// Шаг 2: Дожидаемся, пока все асинхронные задачи будут успешно отправлены в пул.
	// Это ключевая точка синхронизации, устраняющая состояние гонки.
	b.dispatchWg.Wait()

	// Шаг 3: Теперь, когда все задачи гарантированно находятся в очереди,
	// останавливаем пул воркеров. Он обработает все, что есть в очереди, и завершится.
	b.pool.stop()

	return nil
}

// dispatch выполняет вызов одного подписчика.
// Он решает, выполнить вызов синхронно или отправить его в пул воркеров.
// Каждый вызов происходит в отдельной горутине, чтобы не блокировать
// цикл диспетчеризации и других подписчиков.
func (b *Bus[T]) dispatch(ctx context.Context, event Event, sub *subscription) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		if sub.isAsync {
			// Для асинхронных подписчиков задача отправляется в пул воркеров.
			// Мы используем отдельную WaitGroup для отслеживания именно операций отправки,
			// чтобы в Shutdown можно было дождаться их завершения перед остановкой пула.
			b.dispatchWg.Add(1)
			defer b.dispatchWg.Done()
			if ok := b.pool.submit(ctx, event, sub); !ok {
				b.opts.logger.Warnf("Не удалось отправить асинхронную задачу в пул: топик %s", sub.topic)
			}
		} else {
			// Для синхронных подписчиков задача выполняется немедленно в текущей горутине.
			b.pool.ProcessSync(ctx, event, sub)
		}
	}()
}

// wrapHandler использует рефлексию для создания универсальной обертки
// вокруг типизированного обработчика.
// Это позволяет пользователям передавать функции вида `func(ctx, MyEvent)`
// напрямую в Subscribe, без ручного приведения типов.
func wrapHandler(handler any) (EventHandler[Event], error) {
	handlerType := reflect.TypeOf(handler)

	// 1. Проверка, что обработчик является функцией.
	if handlerType.Kind() != reflect.Func {
		return nil, fmt.Errorf("обработчик не является функцией, получен тип: %T", handler)
	}

	// 2. Проверка сигнатуры: количество входных аргументов.
	if handlerType.NumIn() != 2 {
		return nil, fmt.Errorf("обработчик должен принимать 2 аргумента (context.Context, Event), а принимает %d", handlerType.NumIn())
	}

	// 3. Проверка сигнатуры: количество возвращаемых значений.
	if handlerType.NumOut() != 1 {
		return nil, fmt.Errorf("обработчик должен возвращать 1 значение (error), а возвращает %d", handlerType.NumOut())
	}

	// 4. Проверка типов аргументов и возвращаемого значения.
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	eventType := reflect.TypeOf((*Event)(nil)).Elem()
	errorType := reflect.TypeOf((*error)(nil)).Elem()

	if !handlerType.In(0).Implements(ctxType) {
		return nil, fmt.Errorf("первый аргумент обработчика должен быть context.Context, а не %s", handlerType.In(0))
	}
	if !handlerType.In(1).Implements(eventType) {
		return nil, fmt.Errorf("второй аргумент обработчика должен реализовывать интерфейс Event, а %s - нет", handlerType.In(1))
	}
	if !handlerType.Out(0).Implements(errorType) {
		return nil, fmt.Errorf("возвращаемое значение должно быть типом error, а не %s", handlerType.Out(0))
	}

	// 5. Создание обертки.
	handlerValue := reflect.ValueOf(handler)
	specificEventType := handlerType.In(1)

	wrapper := func(ctx context.Context, e Event) error {
		// Проверяем, соответствует ли тип пришедшего события
		// тому, который ожидает обработчик.
		eventValue := reflect.ValueOf(e)
		if !eventValue.Type().AssignableTo(specificEventType) {
			// Тип не совпадает, игнорируем событие.
			// Это ожидаемое поведение, если на один топик подписаны
			// обработчики для разных, но связанных типов событий.
			return nil
		}

		// Вызываем исходный, типизированный обработчик.
		args := []reflect.Value{reflect.ValueOf(ctx), eventValue}
		results := handlerValue.Call(args)

		// Обрабатываем результат.
		if errResult := results[0].Interface(); errResult != nil {
			return errResult.(error)
		}
		return nil
	}

	return wrapper, nil
}
