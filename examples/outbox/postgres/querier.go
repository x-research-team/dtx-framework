package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier определяет интерфейс, который абстрагирует выполнение SQL-запросов.
// Он совместим как с *pgx.Pool, так и с pgx.Tx, что позволяет использовать
// хранилище как в рамках транзакции, так и без нее.
type Querier interface {
	// Exec выполняет SQL-запрос, который не возвращает строк.
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)

	// Query выполняет SQL-запрос и возвращает результат в виде pgx.Rows.
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)

	// QueryRow выполняет SQL-запрос и возвращает одну строку результата.
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}
