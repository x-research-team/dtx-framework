package event

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-reflect"
	"github.com/google/uuid"
)

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

// LocalProvider — это реализация интерфейса Provider, которая обрабатывает
// события локально, в рамках одного процесса. Он использует внутренний
// пул воркеров для асинхронной обработки и потокобезопасный кеш для
// оптимизации производительности.
type LocalProvider struct {
	subscribers map[string][]*subscription
	mu          sync.RWMutex
	cache       sync.Map
	opts        busOptions
	pool        *workerPool
	wg          sync.WaitGroup
	dispatchWg  sync.WaitGroup
	slicePool   sync.Pool
}

// NewLocalProvider создает новый экземпляр LocalProvider.
func NewLocalProvider(opts busOptions) (*LocalProvider, error) {
	pool := newWorkerPool(opts.workerMin, opts.workerMax, opts.queueSize, opts.logger)
	pool.start()

	lp := &LocalProvider{
		subscribers: make(map[string][]*subscription),
		opts:        opts,
		pool:        pool,
	}
	lp.slicePool.New = func() any {
		s := make([]*subscription, 0, 10)
		return &s
	}
	return lp, nil
}

// Publish публикует событие для всех подписчиков.
func (lp *LocalProvider) Publish(ctx context.Context, event Event) error {
	topic := event.Topic()
	var subsToProcess []*subscription

	if cachedSubs, ok := lp.cache.Load(topic); ok {
		subsToProcess = cachedSubs.([]*subscription)
	} else {
		lp.mu.RLock()
		subsSlicePtr := lp.slicePool.Get().(*[]*subscription)
		if subs, ok := lp.subscribers[topic]; ok {
			*subsSlicePtr = append(*subsSlicePtr, subs...)
		}
		lp.mu.RUnlock()

		subsToProcess = *subsSlicePtr
		lp.cache.Store(topic, subsToProcess)
	}

	for _, sub := range subsToProcess {
		lp.dispatch(ctx, event, sub)
	}

	return nil
}

// Subscribe подписывает обработчик на события.
func (lp *LocalProvider) Subscribe(topic string, handler any, opts ...SubscribeOption[Event]) (unsubscribe func(), err error) {
	wrappedHandler, err := wrapHandler(handler)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания обертки обработчика: %w", err)
	}

	subOpts := subscriptionOptions[Event]{}
	for _, opt := range opts {
		opt(&subOpts)
	}

	finalHandler := wrappedHandler
	for i := len(subOpts.middleware) - 1; i >= 0; i-- {
		finalHandler = subOpts.middleware[i](finalHandler)
	}

	sub := &subscription{
		id:           uuid.NewString(),
		topic:        topic,
		handler:      finalHandler,
		isAsync:      subOpts.isAsync,
		errorHandler: subOpts.errorHandler,
	}

	lp.mu.Lock()
	defer lp.mu.Unlock()

	lp.subscribers[topic] = append(lp.subscribers[topic], sub)
	lp.cache.Delete(topic)

	return func() {
		lp.mu.Lock()
		defer lp.mu.Unlock()

		subs := lp.subscribers[topic]
		for i, s := range subs {
			if s.id == sub.id {
				lp.subscribers[topic] = append(subs[:i], subs[i+1:]...)
				lp.cache.Delete(topic)
				break
			}
		}
	}, nil
}

// Shutdown корректно завершает работу провайдера.
func (lp *LocalProvider) Shutdown(ctx context.Context) error {
	lp.wg.Wait()
	lp.dispatchWg.Wait()
	lp.pool.stop()
	return nil
}

func (lp *LocalProvider) dispatch(ctx context.Context, event Event, sub *subscription) {
	lp.wg.Add(1)
	go func() {
		defer lp.wg.Done()

		if sub.isAsync {
			lp.dispatchWg.Add(1)
			defer lp.dispatchWg.Done()
			if ok := lp.pool.submit(ctx, event, sub); !ok {
				lp.opts.logger.Warnf("Не удалось отправить асинхронную задачу в пул: топик %s", sub.topic)
			}
		} else {
			lp.pool.ProcessSync(ctx, event, sub)
		}
	}()
}

// wrapHandler использует рефлексию для создания универсальной обертки
// вокруг типизированного обработчика.
func wrapHandler(handler any) (EventHandler[Event], error) {
	handlerType := reflect.TypeOf(handler)

	if handlerType.Kind() != reflect.Func {
		return nil, fmt.Errorf("обработчик не является функцией, получен тип: %T", handler)
	}
	if handlerType.NumIn() != 2 {
		return nil, fmt.Errorf("обработчик должен принимать 2 аргумента (context.Context, Event), а принимает %d", handlerType.NumIn())
	}
	if handlerType.NumOut() != 1 {
		return nil, fmt.Errorf("обработчик должен возвращать 1 значение (error), а возвращает %d", handlerType.NumOut())
	}

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

	handlerValue := reflect.ValueOf(handler)
	specificEventType := handlerType.In(1)

	wrapper := func(ctx context.Context, e Event) error {
		eventValue := reflect.ValueOf(e)
		if !eventValue.Type().AssignableTo(specificEventType) {
			return nil
		}

		args := []reflect.Value{reflect.ValueOf(ctx), eventValue}
		results := handlerValue.Call(args)

		if errResult := results[0].Interface(); errResult != nil {
			return errResult.(error)
		}
		return nil
	}

	return wrapper, nil
}