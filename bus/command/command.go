package command

import "context"

// Command представляет собой интерфейс-маркер для команды, параметризованный
// типом возвращаемого значения R.
// Каждая команда - это уникальный запрос на выполнение операции.
type Command[R any] interface{}

// CommandHandler определяет строго типизированную функцию-обработчик для команды C,
// которая возвращает результат типа R.
type CommandHandler[C Command[R], R any] func(ctx context.Context, cmd C) (R, error)

// Metadatable - это интерфейс для команд, которые могут переносить метаданные.
// Это используется для сквозной передачи данных, таких как идентификаторы трассировки.
type Metadatable interface {
	Metadata() map[string]string
}