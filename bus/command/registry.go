package command

import (
	"context"
	"fmt"
	"sync"
)

// Registry - это потокобезопасный реестр для управления экземплярами диспетчеров.
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

// Dispatcher возвращает строго типизированный экземпляр диспетчера для указанного имени команды.
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

	if dispatcher, exists := r.dispatchers[commandName]; exists {
		if typedDispatcher, ok := dispatcher.(IDispatcher[C, R]); ok {
			return typedDispatcher, nil
		}
		return nil, fmt.Errorf("диспетчер для команды '%s' уже существует с другим типом", commandName)
	}

	newDispatcher, err := NewDispatcher(opts...)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать новый диспетчер: %w", err)
	}
	r.dispatchers[commandName] = newDispatcher

	return newDispatcher, nil
}

// Shutdown корректно завершает работу всех зарегистрированных диспетчеров.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, dispatcher := range r.dispatchers {
		if d, ok := dispatcher.(interface{ Shutdown(context.Context) error }); ok {
			if err := d.Shutdown(ctx); err != nil {
				// В реальном приложении здесь должно быть логирование.
				fmt.Printf("ошибка при завершении работы диспетчера '%s': %v\n", name, err)
			}
		}
	}

	return nil
}