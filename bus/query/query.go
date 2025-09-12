package query

import "context"

// Query представляет собой интерфейс-маркер для запроса, параметризованный
// типом возвращаемого значения R.
// Каждый запрос - это уникальный, идемпотентный запрос на получение данных.
type Query[R any] interface{}

// QueryHandler определяет строго типизированную функцию-обработчик для запроса Q,
// которая возвращает результат типа R.
type QueryHandler[Q Query[R], R any] func(ctx context.Context, q Q) (R, error)

// Metadatable определяет интерфейс для объектов, которые могут нести метаданные.
type Metadatable interface {
	Metadata() map[string]string
}