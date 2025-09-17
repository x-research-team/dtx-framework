package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/x-research-team/dtx-framework/bus/outbox"
)

const (
	// SQL-запрос для создания таблицы outbox.
	// Индекс по статусу и времени создания для быстрой выборки ожидающих сообщений.
	createTableQuery = `
CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    topic VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB,
    status VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_outbox_status_created_at ON outbox (status, created_at);
`

	// SQL-запрос для вставки нового сообщения.
	insertMessageQuery = `
INSERT INTO outbox (id, topic, payload, metadata, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6);
`

	// SQL-запрос для выборки необработанных сообщений.
	// FOR UPDATE SKIP LOCKED позволяет нескольким ретрансляторам работать параллельно,
	// не блокируя друг друга и не выбирая одни и те же сообщения.
	fetchMessagesQuery = `
SELECT id, topic, payload, metadata, status, created_at, processed_at
FROM outbox
WHERE status = $1
ORDER BY created_at
LIMIT $2
FOR UPDATE SKIP LOCKED;
`

	// SQL-запрос для пометки сообщений как обработанных.
	markProcessedQuery = `
UPDATE outbox
SET status = $1, processed_at = $2
WHERE id = ANY($3);
`
)

// PostgresStorage представляет собой реализацию хранилища outbox для PostgreSQL.
type PostgresStorage struct {
	pool *pgxpool.Pool
}

// NewPostgresStorage создает новый экземпляр PostgresStorage.
// Он также выполняет миграцию, создавая необходимую таблицу, если она не существует.
func NewPostgresStorage(ctx context.Context, pool *pgxpool.Pool) (*PostgresStorage, error) {
	if _, err := pool.Exec(ctx, createTableQuery); err != nil {
		return nil, fmt.Errorf("не удалось создать таблицу outbox: %w", err)
	}
	return &PostgresStorage{pool: pool}, nil
}

// Save сохраняет сообщение в хранилище, используя предоставленный Querier (транзакцию или пул).
func (s *PostgresStorage) Save(ctx context.Context, q Querier, msg *outbox.Message) error {
	metadata, err := json.Marshal(msg.Metadata)
	if err != nil {
		return fmt.Errorf("не удалось сериализовать метаданные: %w", err)
	}

	_, err = q.Exec(ctx, insertMessageQuery,
		msg.ID,
		msg.Topic,
		msg.Payload,
		metadata,
		msg.Status,
		msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("не удалось сохранить сообщение в outbox: %w", err)
	}

	return nil
}

// Fetch извлекает необработанные сообщения из хранилища.
func (s *PostgresStorage) Fetch(ctx context.Context, limit int) ([]*outbox.Message, error) {
	rows, err := s.pool.Query(ctx, fetchMessagesQuery, outbox.StatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("не удалось извлечь сообщения из outbox: %w", err)
	}
	defer rows.Close()

	messages := make([]*outbox.Message, 0)
	for rows.Next() {
		var msg outbox.Message
		var metadata []byte
		if err := rows.Scan(
			&msg.ID,
			&msg.Topic,
			&msg.Payload,
			&metadata,
			&msg.Status,
			&msg.CreatedAt,
			&msg.ProcessedAt,
		); err != nil {
			return nil, fmt.Errorf("не удалось сканировать сообщение: %w", err)
		}
		if err := json.Unmarshal(metadata, &msg.Metadata); err != nil {
			return nil, fmt.Errorf("не удалось десериализовать метаданные: %w", err)
		}
		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка при итерации по сообщениям: %w", err)
	}

	return messages, nil
}

// MarkProcessed помечает сообщения как обработанные.
func (s *PostgresStorage) MarkProcessed(ctx context.Context, ids ...uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	processedAt := time.Now().UTC()
	_, err := s.pool.Exec(ctx, markProcessedQuery, outbox.StatusProcessed, processedAt, ids)
	if err != nil {
		return fmt.Errorf("не удалось пометить сообщения как обработанные: %w", err)
	}

	return nil
}
