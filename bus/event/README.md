# Шина Событий (`bus/event`)

Этот пакет предоставляет строго типизированную, высокопроизводительную и отказоустойчивую шину событий.

## 🚀 Философия

Архитектура основана на **специализации экземпляра шины**, что обеспечивает **абсолютную типобезопасность на этапе компиляции**. Мы полностью отказались от `any` и рефлексии в публичных API в пользу обобщенных (Generics) типов.

- **Типобезопасность:** 100% ошибок, связанных с несоответствием типов событий и обработчиков, отлавливаются компилятором.
- **Производительность:** Отсутствие рефлексии гарантирует минимальные накладные расходы.
- **Прозрачность API:** Контракты интерфейсов ясны, предсказуемы и самодокументируемы.

## 🛠️ Использование

### 1. Определение Событий

Событие — это любая структура данных. Она не обязана реализовывать какие-либо методы.

```go
// Событие создания пользователя
type UserCreatedEvent struct {
    UserID   string
    Email    string
    CreatedAt time.Time
}

// Событие оплаты заказа
type OrderPaidEvent struct {
    OrderID string
    Amount  float64
}
```

### 2. Получение и Использование Шины

Взаимодействие с шинами происходит через `Registry`, который управляет их жизненным циклом.

```go
import "github.com/your-repo/dtx-framework/bus/event"

// 1. Создайте реестр. Обычно это делается один раз при старте приложения.
registry := event.NewRegistry()

// 2. Получите строго типизированную шину для конкретного события и топика.
userBus, err := event.Bus[UserCreatedEvent](registry, "users.created")
if err != nil {
    // Обработка ошибки
}

// 3. Получите другую шину для другого события.
orderBus, err := event.Bus[OrderPaidEvent](registry, "orders.paid")
if err != nil {
    // Обработка ошибки
}
```

### 3. Подписка на События

Обработчик должен иметь строго типизированную сигнатуру `func(ctx context.Context, event T) error`.

```go
// Синхронная подписка
unsubscribe, err := userBus.Subscribe(
    func(ctx context.Context, e UserCreatedEvent) error {
        fmt.Printf("Обработано событие создания пользователя: %s\n", e.UserID)
        return nil
    },
)
// ...
// Не забудьте отписаться, когда обработчик больше не нужен
defer unsubscribe()
```

### 4. Публикация Событий

Публикация события в шину, типизированную другим событием, приведет к ошибке компиляции.

```go
// Корректная публикация
err = userBus.Publish(context.Background(), UserCreatedEvent{UserID: "user-123"})
// ...

// ОШИБКА КОМПИЛЯЦИИ!
// err = userBus.Publish(context.Background(), OrderPaidEvent{OrderID: "order-456"})
```

### 5. Расширенные Опции Подписки

#### Асинхронная Обработка

Для выполнения обработчика в отдельной горутине используйте опцию `WithAsync`.

```go
bus.Subscribe(
    handler,
    event.WithAsync[UserCreatedEvent](),
)
```

#### Обработка Ошибок

Для централизованной обработки ошибок, возвращаемых обработчиком, используйте `WithErrorHandler`.

```go
bus.Subscribe(
    handler,
    event.WithErrorHandler(func(err error, e UserCreatedEvent) {
        log.Printf("Ошибка при обработке события %v: %v", e, err)
    }),
)
```

#### Промежуточное ПО (Middleware)

Для добавления сквозной функциональности (логирование, метрики, трассировка) используйте `WithMiddleware`.

```go
loggingMiddleware := func(next event.EventHandler[UserCreatedEvent]) event.EventHandler[UserCreatedEvent] {
    return func(ctx context.Context, e UserCreatedEvent) error {
        log.Printf("Начало обработки события: %v", e)
        err := next(ctx, e)
        log.Printf("Конец обработки события: %v", e)
        return err
    }
}

bus.Subscribe(
    handler,
    event.WithMiddleware(loggingMiddleware),
)