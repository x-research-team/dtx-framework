// Package event определяет основные интерфейсы и типы для обобщенной,
// типобезопасной системы шины событий. Пакет спроектирован для высокой
// производительности, гибкости и наблюдаемости при локальном (внутрипроцессном)
// взаимодействии.
package event

import (
	"context"
	"time"
)

// Event определяет минимальный контракт для любого события, которое может быть
// передано через шину.
type Event interface {
	// Topic возвращает имя топика, к которому относится событие.
	// Это позволяет шине маршрутизировать событие, не зная его конкретного типа.
	Topic() string
}

// EventHandler — это тип для функции-обработчика, которая принимает контекст
// и конкретный тип события. Он является обобщенным для обеспечения статической
// безопасности типов.
type EventHandler[T Event] func(ctx context.Context, event T) error

// ErrorHandler — это функция для обработки ошибок, возникших в EventHandler.
type ErrorHandler[T Event] func(err error, event T)

// Middleware — это функция-декоратор для EventHandler.
// Она принимает следующий обработчик в цепочке и возвращает новый обработчик.
type Middleware[T Event] func(next EventHandler[T]) EventHandler[T]

// EventBus определяет основной интерфейс для публикации и подписки на события.
// ПРИМЕЧАНИЕ: Go не поддерживает обобщенные методы в интерфейсах. Исходная
// архитектура определяла обобщенные методы Publish и Subscribe. Чтобы сделать код
// компилируемым, методы теперь принимают базовый тип `Event` или `any`.
// Безопасность типов будет обеспечиваться на уровне реализации с использованием
// рефлексии и утверждений типов, сохраняя основной принцип проектирования —
// безопасность типов во время компиляции для вызывающей стороны, где это
// возможно (например, через обобщенные вспомогательные функции вне интерфейса).
type EventBus[T Event] interface {
	// Publish публикует событие в шину.
	// Контекст передается для поддержки трассировки и отмены.
	// Метод возвращает ошибку только в случае критического сбоя самой шины;
	// ошибки обработчиков обрабатываются асинхронно.
	Publish(ctx context.Context, event T) error

	// Subscribe подписывает обработчик на события определенного топика.
	// Обработчик должен быть функцией типа `EventHandler[T]`, где T — это
	// конкретный тип события для данного топика. Это проверяется во время выполнения.
	// Метод возвращает функцию для отписки и ошибку, если подписка не удалась.
	Subscribe(topic string, handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error)

	// Shutdown корректно завершает работу шины, ожидая завершения всех
	// выполняющихся асинхронных обработчиков.
	Shutdown(ctx context.Context) error
}

// Provider определяет контракт для всех сменных механизмов доставки событий.
// Этот интерфейс является ключевым элементом архитектуры, позволяя
// подменять реализацию (например, с локальной на Kafka или NATS) без
// изменения кода, использующего шину.
type Provider interface {
	// Publish публикует событие в шину. Реализация должна гарантировать
	// атомарность и, в зависимости от конфигурации, надежность доставки.
	Publish(ctx context.Context, event Event) error

	// Subscribe подписывает обработчик на указанный топик.
	// Возвращает функцию для отписки, что позволяет динамически управлять
	// подписками. Опции (opts) позволяют тонко настраивать поведение
	// подписки, например, указывать группу подписчиков или параметры
	// ретраев.
	Subscribe(topic string, handler any, opts ...SubscribeOption[Event]) (unsubscribe func(), err error)

	// Shutdown корректно завершает работу провайдера, освобождая все ресурсы.
	// Этот метод должен дождаться завершения обработки всех активных задач
	// и закрыть все сетевые соединения.
	Shutdown(ctx context.Context) error
}

// busOptions определяет набор параметров для конфигурации экземпляра шины событий.
// Эта структура не экспортируется и управляется исключительно через функциональные
// опции типа BusOption, что обеспечивает безопасную и предсказуемую настройку.
type busOptions struct {
	// logger определяет экземпляр логгера для вывода внутренней информации
	// о работе шины. Если не задан, используется noopLogger.
	logger Logger
	// metrics определяет экземпляр сборщика метрик для отслеживания
	// ключевых показателей производительности. Если не задан, используется noopMetrics.
	metrics Metrics
	// workerMin задает минимальное количество активных воркеров в пуле.
	// Значение по умолчанию: 1.
	workerMin int
	// workerMax задает максимальное количество активных воркеров в пуле.
	// Значение по умолчанию: 10.
	workerMax int
	// queueSize определяет размер буферизированного канала для задач воркеров.
	// Значение по умолчанию: 100.
	queueSize int
	provider  Provider // Поле для хранения сменного провайдера.
}

// subscriptionOptions определяет набор параметров для конфигурации конкретной подписки.
// Управляется через функциональные опции типа SubscribeOption.
type subscriptionOptions[T Event] struct {
	// isAsync указывает, должен ли обработчик выполняться асинхронно.
	// По умолчанию обработка синхронна.
	isAsync bool
	// errorHandler задает пользовательскую функцию для обработки ошибок,
	// возникающих в EventHandler.
	errorHandler ErrorHandler[T]
	// middleware содержит цепочку функций-декораторов, которые будут
	// применены к обработчику события.
	middleware []Middleware[T]
}

// BusOption — это функциональная опция для настройки экземпляра шины.
type BusOption func(*busOptions)

// SubscribeOption — это функциональная опция для настройки подписки.
type SubscribeOption[T Event] func(*subscriptionOptions[T])

// Logger определяет интерфейс для структурированного логирования.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Metrics определяет интерфейс для сбора ключевых метрик работы шины.
type Metrics interface {
	// IncEventsPublished увеличивает счетчик опубликованных событий.
	IncEventsPublished(topic string)

	// IncEventsHandled увеличивает счетчик обработанных событий.
	// Метка success позволяет различать успешные и неуспешные обработки.
	IncEventsHandled(topic string, success bool)

	// ObserveEventHandleDuration записывает длительность обработки события.
	// Используется для гистограмм или сводных метрик.
	ObserveEventHandleDuration(topic string, duration time.Duration)
}

// --- Функции для создания опций ---

// WithProvider устанавливает кастомный провайдер для шины.
// Это ключевая опция, позволяющая реализовать паттерн "Стратегия".
func WithProvider(p Provider) BusOption {
	return func(o *busOptions) {
		o.provider = p
	}
}

// WithLogger устанавливает пользовательский логгер для шины.
func WithLogger(logger Logger) BusOption {
	return func(o *busOptions) {
		o.logger = logger
	}
}

// WithMetrics устанавливает пользовательский сборщик метрик для шины.
func WithMetrics(metrics Metrics) BusOption {
	return func(o *busOptions) {
		o.metrics = metrics
	}
}

// WithWorkerPoolConfig настраивает параметры пула горутин для асинхронных обработчиков.
func WithWorkerPoolConfig(minWorkers, maxWorkers, queueSize int) BusOption {
	return func(o *busOptions) {
		o.workerMin = minWorkers
		o.workerMax = maxWorkers
		o.queueSize = queueSize
	}
}

// WithAsync — опция, включающая асинхронный режим обработки для подписчика.
func WithAsync[T Event]() SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.isAsync = true
	}
}

// WithErrorHandler — опция, позволяющая задать пользовательский обработчик ошибок.
func WithErrorHandler[T Event](handler ErrorHandler[T]) SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.errorHandler = handler
	}
}

// WithMiddleware добавляет локальные middleware, которые применяются только к данной подписке.
func WithMiddleware[T Event](mw ...Middleware[T]) SubscribeOption[T] {
	return func(o *subscriptionOptions[T]) {
		o.middleware = append(o.middleware, mw...)
	}
}
