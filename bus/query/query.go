package query

import "context"

// Query представляет собой интерфейс-маркер для запроса, параметризованный
// типом возвращаемого значения R.
// Каждый запрос - это уникальный, идемпотентный запрос на получение данных.
type Query[R any] interface{}

// QueryHandler определяет строго типизированную функцию-обработчик для запроса Q,
// которая возвращает результат типа R.
type QueryHandler[Q Query[R], R any] func(ctx context.Context, q Q) (R, error)

// Middleware определяет тип функции-декоратора для QueryHandler.
// Он позволяет добавлять сквозную функциональность (логирование, метрики, кэширование)
// вокруг основной логики обработчика.
type Middleware[Q Query[R], R any] func(next QueryHandler[Q, R]) QueryHandler[Q, R]

// config содержит конфигурацию для диспетчера, включая middlewares.
type config[Q Query[R], R any] struct {
	middlewares []Middleware[Q, R]
}

// Option определяет тип функции для конфигурации диспетчера.
type Option[Q Query[R], R any] func(*config[Q, R])

// WithMiddleware создает опцию, которая добавляет новый middleware в цепочку.
func WithMiddleware[Q Query[R], R any](m Middleware[Q, R]) Option[Q, R] {
	return func(c *config[Q, R]) {
		c.middlewares = append(c.middlewares, m)
	}
}