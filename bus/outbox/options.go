package outbox

import "github.com/x-research-team/dtx-framework/bus/event"

// Option определяет функцию для конфигурации OutboxMiddleware.
type Option[T event.Event] func(*OutboxMiddleware[T])

// WithTopic устанавливает топик, в который будут сохраняться сообщения.
// Этот топик будет использоваться для всех событий, проходящих через данный middleware.
func WithTopic[T event.Event](topic string) Option[T] {
	return func(m *OutboxMiddleware[T]) {
		m.topic = topic
	}
}