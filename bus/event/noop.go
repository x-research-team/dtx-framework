// Package event определяет основные интерфейсы и типы для обобщенной,
// типобезопасной системы шины событий. Пакет спроектирован для высокой
// производительности, гибкости и наблюдаемости при локальном (внутрипроцессном)
// взаимодействии.
package event

import "time"

// noopLogger — это реализация-заглушка для интерфейса Logger.
// Она не выполняет никаких действий и используется по умолчанию, если
// пользовательский логгер не предоставлен.
type noopLogger struct{}

// Debugf ничего не делает.
func (l *noopLogger) Debugf(format string, args ...any) {}

// Infof ничего не делает.
func (l *noopLogger) Infof(format string, args ...any) {}

// Warnf ничего не делает.
func (l *noopLogger) Warnf(format string, args ...any) {}

// Errorf ничего не делает.
func (l *noopLogger) Errorf(format string, args ...any) {}

// noopMetrics — это реализация-заглушка для интерфейса Metrics.
// Она не выполняет никаких действий и используется по умолчанию, если
// пользовательский сборщик метрик не предоставлен.
type noopMetrics struct{}

// IncEventsPublished ничего не делает.
func (m *noopMetrics) IncEventsPublished(topic string) {}

// IncEventsHandled ничего не делает.
func (m *noopMetrics) IncEventsHandled(topic string, success bool) {}

// ObserveEventHandleDuration ничего не делает.
func (m *noopMetrics) ObserveEventHandleDuration(topic string, duration time.Duration) {}
