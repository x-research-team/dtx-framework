# Event Bus

Высокопроизводительная, потокобезопасная, статически типизированная локальная шина событий для Go.

## Основные возможности

*   **Статическая типизация:** Использование дженериков (Generics) и рефлексии для обеспечения безопасности типов на этапе компиляции и во время выполнения. Вы можете передавать строго типизированные обработчики, например `func(ctx, UserCreatedEvent)`, без необходимости ручного приведения типов.
*   **Потокобезопасность:** Гарантированная безопасность при работе в многопоточной среде.
*   **Синхронная и асинхронная обработка:** Поддержка как синхронных, так и асинхронных обработчиков событий с помощью внутреннего пула воркеров.
*   **Промежуточное ПО (Middleware):** Возможность добавления middleware для расширения функциональности обработчиков.
*   **Гибкая конфигурация:** Настройка шины и подписок с помощью паттерна "функциональные опции".
*   **Наблюдаемость:** Встроенные интерфейсы для логирования и сбора метрик.
*   **Надежность:** Механизм корректного завершения работы, гарантирующий обработку всех событий.
*   **Поддержка CQRS:** Идеально подходит для реализации паттерна CQRS в ваших приложениях.

## Установка

```bash
go get github.com/X-Research-Team/dtx-framework/bus/event
```

## Пример использования

Ниже приведен полный пример, демонстрирующий основные возможности шины событий, включая использование строго типизированных обработчиков.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/X-Research-Team/dtx-framework/bus/event"
)

// 1. Определение событий

// UserCreatedEvent - событие создания пользователя
type UserCreatedEvent struct {
	UserID string
	Email  string
}

func (e UserCreatedEvent) Topic() string {
	return "user.created"
}

// OrderPlacedEvent - событие размещения заказа
type OrderPlacedEvent struct {
	OrderID string
	Amount  float64
}

func (e OrderPlacedEvent) Topic() string {
	return "order.placed"
}

// 2. Создание и настройка шины

func main() {
	// Создаем новую шину событий.
	// NewBus возвращает *Bus[event.Event], что позволяет публиковать
	// и подписываться на любой тип, реализующий интерфейс event.Event.
	bus := event.NewBus()
	defer bus.Shutdown(context.Background())

	// 3. Создание строго типизированных обработчиков

	// Синхронный обработчик для UserCreatedEvent.
	// Обратите внимание: тип второго аргумента - UserCreatedEvent, а не event.Event.
	// Ручное приведение типов больше не требуется.
	userCreatedHandler := func(ctx context.Context, ev UserCreatedEvent) error {
		fmt.Printf("[Синхронный обработчик] Пользователь создан: ID=%s, Email=%s\n", ev.UserID, ev.Email)
		return nil
	}

	// Асинхронный обработчик для OrderPlacedEvent.
	orderPlacedHandler := func(ctx context.Context, ev OrderPlacedEvent) error {
		fmt.Printf("[Асинхронный обработчик] Заказ размещен: ID=%s, Сумма=%.2f\n", ev.OrderID, ev.Amount)
		time.Sleep(100 * time.Millisecond) // Имитация долгой работы
		return nil
	}

	// Обработчик ошибок для конкретного типа события.
	orderErrorHandler := func(err error, e OrderPlacedEvent) {
		log.Printf("Ошибка при обработке OrderPlacedEvent %v: %v", e, err)
	}

	// 4. Подписка на события

	// Подписываем синхронный обработчик.
	// Тип обработчика `func(context.Context, UserCreatedEvent) error` корректно распознается.
	unsubscribeUser, err := bus.Subscribe(UserCreatedEvent{}.Topic(), userCreatedHandler)
	if err != nil {
		log.Fatalf("Не удалось подписаться на событие 'user.created': %v", err)
	}
	defer unsubscribeUser()

	// Подписываем асинхронный обработчик с обработчиком ошибок.
	// Опции WithAsync и WithErrorHandler также являются типобезопасными.
	unsubscribeOrder, err := bus.Subscribe(
		OrderPlacedEvent{}.Topic(),
		orderPlacedHandler,
		event.WithAsync[OrderPlacedEvent](),
		event.WithErrorHandler[OrderPlacedEvent](orderErrorHandler),
	)
	if err != nil {
		log.Fatalf("Не удалось подписаться на событие 'order.placed': %v", err)
	}
	defer unsubscribeOrder()

	// 5. Публикация событий

	fmt.Println("Публикация событий...")
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := bus.Publish(context.Background(), UserCreatedEvent{UserID: "user-123", Email: "test@example.com"})
		if err != nil {
			log.Printf("Ошибка публикации UserCreatedEvent: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		err := bus.Publish(context.Background(), OrderPlacedEvent{OrderID: "order-456", Amount: 99.99})
		if err != nil {
			log.Printf("Ошибка публикации OrderPlacedEvent: %v", err)
		}
	}()

	wg.Wait()
	fmt.Println("Все события опубликованы.")

	// 6. Корректное завершение работы
	// defer bus.Shutdown() гарантирует, что все асинхронные обработчики завершат свою работу.
	fmt.Println("Завершение работы...")
}
```

## API

### `event.Event`

Основной интерфейс, который должно реализовывать любое событие.

```go
type Event interface {
    Topic() string
}
```

### `event.EventHandler[T]`

Тип для функции-обработчика событий. `T` должен быть конкретным типом, реализующим `event.Event`.

```go
type EventHandler[T Event] func(ctx context.Context, event T) error
```

### `event.EventBus`

Основной интерфейс шины событий. Реализация `*Bus[T]` является обобщенной, но для удобства использования `NewBus` возвращает `*Bus[Event]`, что позволяет работать с любыми событиями.

```go
// Полный интерфейс не приводится для краткости. Ключевые методы:
Publish(ctx context.Context, event Event) error
Subscribe(topic string, handler any, opts ...SubscribeOption[Event]) (unsubscribe func(), err error)
Shutdown(ctx context.Context) error
```

### `event.NewBus(...)`

Функция-конструктор для создания нового экземпляра шины.

```go
func NewBus(opts ...BusOption) *Bus[Event]
```

### `Subscribe`

Метод `Subscribe` принимает `any` в качестве обработчика, но использует рефлексию для обеспечения типобезопасности во время выполнения. Вы можете передавать функции вида `func(ctx, YourEventType) error`, и шина автоматически создаст обертку.

### Опции

*   **Для шины (`BusOption`):**
    *   `WithLogger(Logger)`: Устанавливает пользовательский логгер.
    *   `WithMetrics(Metrics)`: Устанавливает пользовательский сборщик метрик.
    *   `WithWorkerPoolConfig(min, max, queueSize)`: Настраивает пул воркеров.
*   **Для подписки (`SubscribeOption[T]`):**
    *   `WithAsync[T]()`: Включает асинхронную обработку. `T` должен соответствовать типу события в обработчике.
    *   `WithErrorHandler[T](ErrorHandler[T])`: Задает обработчик ошибок. `T` должен соответствовать типу события.
    *   `WithMiddleware[T](...Middleware[T])`: Добавляет middleware. `T` должен соответствовать типу события.