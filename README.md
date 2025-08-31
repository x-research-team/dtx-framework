# Event Bus

Высокопроизводительная, потокобезопасная, статически типизированная локальная шина событий для Go.

## Основные возможности

*   **Статическая типизация:** Использование дженериков (Generics) для обеспечения безопасности типов на этапе компиляции.
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

Ниже приведен полный пример, демонстрирующий основные возможности шины событий.

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
	// Создаем новую шину событий
	bus := event.NewBus()
	defer bus.Shutdown(context.Background())

	// 3. Создание обработчиков

	// Синхронный обработчик для UserCreatedEvent
	userCreatedHandler := func(ctx context.Context, e event.Event) error {
		if ev, ok := e.(UserCreatedEvent); ok {
			fmt.Printf("[Синхронный обработчик] Пользователь создан: ID=%s, Email=%s\n", ev.UserID, ev.Email)
		}
		return nil
	}

	// Асинхронный обработчик для OrderPlacedEvent
	orderPlacedHandler := func(ctx context.Context, e event.Event) error {
		if ev, ok := e.(OrderPlacedEvent); ok {
			fmt.Printf("[Асинхронный обработчик] Заказ размещен: ID=%s, Сумма=%.2f\n", ev.OrderID, ev.Amount)
			time.Sleep(100 * time.Millisecond) // Имитация долгой работы
		}
		return nil
	}

	// Обработчик ошибок
	errorHandler := func(err error, e event.Event) {
		log.Printf("Ошибка при обработке события %T: %v", e, err)
	}

	// 4. Подписка на события

	// Подписываем синхронный обработчик
	unsubscribeUser, err := bus.Subscribe(
		UserCreatedEvent{}.Topic(),
		userCreatedHandler,
	)
	if err != nil {
		log.Fatalf("Не удалось подписаться на событие 'user.created': %v", err)
	}
	defer unsubscribeUser()

	// Подписываем асинхронный обработчик с обработчиком ошибок
	unsubscribeOrder, err := bus.Subscribe(
		OrderPlacedEvent{}.Topic(),
		orderPlacedHandler,
		event.WithAsync[event.Event](),
		event.WithErrorHandler[event.Event](errorHandler),
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
	// defer bus.Shutdown() гарантирует, что все асинхронные обработчики завершат свою работу
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

Тип для функции-обработчика событий.

```go
type EventHandler[T Event] func(ctx context.Context, event T) error
```

### `event.EventBus[T]`

Основной интерфейс шины событий.

```go
type EventBus[T Event] interface {
    Publish(ctx context.Context, event T) error
    Subscribe(topic string, handler EventHandler[T], opts ...SubscribeOption[T]) (unsubscribe func(), err error)
    Shutdown(ctx context.Context) error
}
```

### `event.NewBus(...)`

Функция-конструктор для создания нового экземпляра шины.

```go
func NewBus(opts ...BusOption) *Bus[Event]
```

### Опции

*   **Для шины (`BusOption`):**
    *   `WithLogger(Logger)`: Устанавливает пользовательский логгер.
    *   `WithMetrics(Metrics)`: Устанавливает пользовательский сборщик метрик.
    *   `WithWorkerPoolConfig(min, max, queueSize)`: Настраивает пул воркеров.
*   **Для подписки (`SubscribeOption`):**
    *   `WithAsync[T]()`: Включает асинхронную обработку.
    *   `WithErrorHandler[T](ErrorHandler[T])`: Задает обработчик ошибок.
    *   `WithMiddleware[T](...Middleware[T])`: Добавляет middleware.