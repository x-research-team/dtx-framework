package query

import (
	"context"
	"fmt"
	"sync"
)

// Registry - это потокобезопасный реестр для управления экземплярами диспетчеров.
// Он гарантирует, что для каждого имени запроса существует только один экземпляр
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
// указанного имени запроса.
// Если диспетчер для данного имени уже существует, он будет возвращен.
// В противном случае будет создан, сохранен в реестре и возвращен новый экземпляр.
// Функция обеспечивает потокобезопасность и предотвращает состояние гонки
// при создании нескольких диспетчеров для одного и того же запроса.
func Dispatcher[Q Query[R], R any](r *Registry, queryName string, opts ...Option[Q, R]) (IDispatcher[Q, R], error) {
	r.mu.RLock()
	dispatcher, exists := r.dispatchers[queryName]
	r.mu.RUnlock()

	if exists {
		if typedDispatcher, ok := dispatcher.(IDispatcher[Q, R]); ok {
			return typedDispatcher, nil
		}
		return nil, fmt.Errorf("диспетчер для запроса '%s' уже существует с другим типом", queryName)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Повторная проверка на случай, если диспетчер был создан во время ожидания блокировки.
	if dispatcher, exists := r.dispatchers[queryName]; exists {
		if typedDispatcher, ok := dispatcher.(IDispatcher[Q, R]); ok {
			return typedDispatcher, nil
		}
		return nil, fmt.Errorf("диспетчер для запроса '%s' уже существует с другим типом", queryName)
	}

	newDispatcher, err := NewDispatcher(opts...)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать новый диспетчер: %w", err)
	}
	r.dispatchers[queryName] = newDispatcher

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
