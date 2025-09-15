package event

import (
	"context"
	"fmt"
	"sync"
)

// Registry - это потокобезопасный реестр для управления экземплярами шин событий.
// Он гарантирует, что для каждого топика существует только один экземпляр шины
// определенного типа.
type Registry struct {
	mu    sync.RWMutex
	buses map[string]any
}

// NewRegistry создает новый экземпляр реестра шин.
func NewRegistry() *Registry {
	return &Registry{
		buses: make(map[string]any),
	}
}

// Bus возвращает строго типизированный экземпляр шины для указанного топика.
// Эта функция является идиоматичным способом работы с реестром в Go,
// обходя ограничение на отсутствие обобщенных методов.
func Bus[T Event](r *Registry, topic string, opts ...Option[T]) (IBus[T], error) {
	r.mu.RLock()
	bus, exists := r.buses[topic]
	r.mu.RUnlock()

	if exists {
		if typedBus, ok := bus.(IBus[T]); ok {
			return typedBus, nil
		}
		return nil, fmt.Errorf("шина для топика '%s' уже существует с другим типом события", topic)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Повторная проверка на случай, если шина была создана во время ожидания блокировки.
	if bus, exists := r.buses[topic]; exists {
		if typedBus, ok := bus.(IBus[T]); ok {
			return typedBus, nil
		}
		return nil, fmt.Errorf("шина для топика '%s' уже существует с другим типом события", topic)
	}

	// Теперь NewBus сам заботится о создании провайдера по умолчанию.
	// Мы просто передаем ему все опции.
	newBus, err := NewBus(topic, opts...)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать шину для топика '%s': %w", topic, err)
	}

	r.buses[topic] = newBus
	return newBus, nil
}

// Shutdown корректно завершает работу всех зарегистрированных шин.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for topic, busInstance := range r.buses {
		if shutdowner, ok := busInstance.(interface {
			Shutdown(context.Context) error
		}); ok {
			if err := shutdowner.Shutdown(ctx); err != nil {
				// В реальном приложении здесь должно быть логирование ошибки.
				fmt.Printf("ошибка при закрытии шины для топика %s: %v\n", topic, err)
			}
		}
	}

	return nil
}
