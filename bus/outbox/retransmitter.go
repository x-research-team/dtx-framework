package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/x-research-team/dtx-framework/bus/event"
)

// RetransmitterOption определяет функцию для конфигурации Retransmitter.
type RetransmitterOption[T event.Event] func(*Retransmitter[T])

// WithInterval устанавливает интервал опроса хранилища.
func WithInterval[T event.Event](interval time.Duration) RetransmitterOption[T] {
	return func(r *Retransmitter[T]) {
		r.interval = interval
	}
}

// WithLimit устанавливает максимальное количество сообщений, извлекаемых за один раз.
func WithLimit[T event.Event](limit int) RetransmitterOption[T] {
	return func(r *Retransmitter[T]) {
		r.limit = limit
	}
}

// WithLogger устанавливает логгер.
func WithLogger[T event.Event](logger *slog.Logger) RetransmitterOption[T] {
	return func(r *Retransmitter[T]) {
		r.logger = logger
	}
}

// Retransmitter - это фоновый процесс для надежной доставки сообщений.
// Он типизирован параметром T, что предполагает, что он работает с одним
// конкретным типом события и соответствующим ему хранилищем Storage[T].
type Retransmitter[T event.Event] struct {
	storage   Storage[T]
	actualBus event.IBus[T]
	ticker    *time.Ticker
	done      chan struct{}
	interval  time.Duration
	limit     int
	logger    *slog.Logger
}

// NewRetransmitter создает новый экземпляр Retransmitter.
func NewRetransmitter[T event.Event](storage Storage[T], actualBus event.IBus[T], opts ...RetransmitterOption[T]) *Retransmitter[T] {
	r := &Retransmitter[T]{
		storage:   storage,
		actualBus: actualBus,
		done:      make(chan struct{}),
		interval:  5 * time.Second, // Значение по умолчанию
		limit:     100,             // Значение по умолчанию
		logger:    slog.Default(),  // Логгер по умолчанию
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Start запускает фоновый процесс.
func (r *Retransmitter[T]) Start() {
	r.ticker = time.NewTicker(r.interval)
	go func() {
		r.logger.Info("Retransmitter запущен")
		for {
			select {
			case <-r.ticker.C:
				if err := r.processBatch(); err != nil {
					r.logger.Error("Ошибка при обработке пакета", "error", err)
				}
			case <-r.done:
				r.logger.Info("Retransmitter остановлен")
				return
			}
		}
	}()
}

// processBatch выполняет один цикл выборки и отправки сообщений.
func (r *Retransmitter[T]) processBatch() error {
	ctx := context.Background()
	messages, err := r.storage.Fetch(ctx, r.limit)
	if err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	r.logger.Info("Извлечено сообщений для ретрансляции", "count", len(messages))

	processedIDs := make([]uuid.UUID, 0, len(messages))
	for _, msg := range messages {
		var eventInstance T
		if err := json.Unmarshal(msg.Payload, &eventInstance); err != nil {
			r.logger.Error("Ошибка десериализации сообщения", "message_id", msg.ID, "error", err)
			continue
		}

		if eventInstance.Metadata() != nil && msg.Metadata != nil {
			for k, v := range msg.Metadata {
				eventInstance.Metadata()[k] = v
			}
		}

		if err := r.actualBus.Publish(ctx, eventInstance); err != nil {
			r.logger.Error("Ошибка публикации события", "message_id", msg.ID, "error", err)
			continue
		}

		processedIDs = append(processedIDs, msg.ID)
	}

	if len(processedIDs) > 0 {
		if err := r.storage.MarkProcessed(ctx, processedIDs...); err != nil {
			return err
		}
		r.logger.Info("Успешно обработано и помечено сообщений", "count", len(processedIDs))
	}

	return nil
}

// Stop останавливает фоновый процесс.
func (r *Retransmitter[T]) Stop() {
	if r.ticker != nil {
		r.ticker.Stop()
	}
	close(r.done)
}