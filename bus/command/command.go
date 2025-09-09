package command

import "context"

// Command представляет собой интерфейс-маркер для команды, параметризованный
// типом возвращаемого значения R.
// Каждая команда - это уникальный запрос на выполнение операции.
type Command[R any] interface{}

// CommandHandler определяет строго типизированную функцию-обработчик для команды C,
// которая возвращает результат типа R.
type CommandHandler[C Command[R], R any] func(ctx context.Context, cmd C) (R, error)

// Middleware определяет тип функции-декоратора для CommandHandler.
// Он позволяет добавлять сквозную функциональность (логирование, метрики, трассировка)
// вокруг основной логики обработчика.
type Middleware[C Command[R], R any] func(next CommandHandler[C, R]) CommandHandler[C, R]