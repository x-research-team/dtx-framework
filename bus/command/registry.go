package command

import (
	"fmt"
	"sync"
)

// Registry - это потокобезопасный реестр для управления экземплярами диспетчеров.
// Он гарантирует, что для каждого имени команды существует только один экземпляр
// диспетчера.
type Registry struct {
	dispatchers map[string]any
	mu          sync.RWMutex
}

// NewRegistry создает новый экземпляр реестра диспетчеров.
func NewRegistry() *Registry {
	return &Registry{
		dispatchers: make(map[string]any),
	}
}

// Dispatcher возвращает строго типизированный экземпляр диспетчера для
// указанного имени команды.
// Если диспетчер для данного имени уже существует, он будет возвращен.
// В противном случае будет создан, сохранен в реестре и возвращен новый экземпляр.
// Функция обеспечивает потокобезопасность и предотвращает состояние гонки
// при создании нескольких диспетчеров для одной и той же команды.
func Dispatcher[C Command[R], R any](r *Registry, commandName string, opts ...Option[C, R]) (IDispatcher[C, R], error) {
	r.mu.RLock()
	dispatcher, exists := r.dispatchers[commandName]
	r.mu.RUnlock()

	if exists {
		if typedDispatcher, ok := dispatcher.(IDispatcher[C, R]); ok {
			return typedDispatcher, nil
		}
		return nil, fmt.Errorf("диспетчер для команды '%s' уже существует с другим типом", commandName)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Повторная проверка на случай, если диспетчер был создан во время ожидания блокировки.
	if dispatcher, exists := r.dispatchers[commandName]; exists {
		if typedDispatcher, ok := dispatcher.(IDispatcher[C, R]); ok {
			return typedDispatcher, nil
		}
		return nil, fmt.Errorf("диспетчер для команды '%s' уже существует с другим типом", commandName)
	}

	newDispatcher := NewDispatcher(opts...)
	r.dispatchers[commandName] = newDispatcher

	return newDispatcher, nil
}