package outbox

import (
	"time"

	"github.com/google/uuid"
)

const (
	// StatusPending означает, что сообщение ожидает обработки.
	StatusPending = "PENDING"
	// StatusProcessed означает, что сообщение было успешно обработано.
	StatusProcessed = "PROCESSED"
)

// Message представляет событие, сохраненное в хранилище outbox.
type Message struct {
	ID          uuid.UUID         // Уникальный идентификатор сообщения
	Topic       string            // Топик назначения в шине
	Payload     []byte            // Сериализованное тело события
	Metadata    map[string]string // Метаданные (для трассировки и т.д.)
	Status      string            // Статус (например, PENDING, PROCESSED)
	CreatedAt   time.Time         // Время создания
	ProcessedAt *time.Time        // Время обработки
}