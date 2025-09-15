package outbox

import (
	"context"

	"github.com/google/uuid"

	"github.com/x-research-team/dtx-framework/bus/event"
)

// Storage определяет контракт для персистентного хранения сообщений outbox.
// Все операции должны быть потокобезопасными.
type Storage[T event.Event] interface {
	// Save сохраняет сообщение в хранилище.
	// Реализация этого метода ОБЯЗАНА извлечь объект транзакции из контекста
	// и выполнить операцию в рамках этой транзакции.
	Save(ctx context.Context, msg *Message) error

	// Fetch извлекает необработанные сообщения из хранилища.
	Fetch(ctx context.Context, limit int) ([]*Message, error)

	// MarkProcessed помечает сообщения как обработанные.
	MarkProcessed(ctx context.Context, ids ...uuid.UUID) error
}