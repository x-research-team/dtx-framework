package command_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/x-research-team/dtx-framework/bus/command"
)

// Тестовая команда для проверки.
type testCommand struct {
	Value string
}

// Тестовая команда для проверки несовпадения типов.
type anotherTestCommand struct {
	Value int
}

// Тестовый обработчик команды.
func testCommandHandler(ctx context.Context, cmd testCommand) (string, error) {
	return "processed: " + cmd.Value, nil
}

// Тест успешной регистрации и выполнения команды.
func TestDispatcher_Success(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер.
	dispatcher := command.NewDispatcher[testCommand, string]()
	err := dispatcher.Register(testCommandHandler)
	require.NoError(t, err, "Регистрация обработчика не должна вызывать ошибку")

	// Отправляем команду.
	cmd := testCommand{Value: "test"}
	result, err := dispatcher.Dispatch(context.Background(), cmd)

	// Проверяем результат.
	require.NoError(t, err, "Выполнение команды не должно вызывать ошибку")
	assert.Equal(t, "processed: test", result, "Результат выполнения команды некорректен")
}

// Тест ошибки при отправке команды без зарегистрированного обработчика.
func TestDispatcher_Dispatch_NoHandler(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер без регистрации обработчика.
	dispatcher := command.NewDispatcher[testCommand, string]()

	// Отправляем команду.
	cmd := testCommand{Value: "test"}
	_, err := dispatcher.Dispatch(context.Background(), cmd)

	// Проверяем ошибку.
	require.Error(t, err, "Выполнение команды без обработчика должно вызывать ошибку")
	assert.Contains(t, err.Error(), "обработчик для команды", "Текст ошибки должен содержать информацию об отсутствующем обработчике")
	assert.Contains(t, err.Error(), "не найден", "Текст ошибки должен содержать информацию о том, что обработчик не найден")
}

// Тест ошибки при повторной регистрации обработчика.
func TestDispatcher_Register_AlreadyRegistered(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер и регистрируем обработчик.
	dispatcher := command.NewDispatcher[testCommand, string]()
	err := dispatcher.Register(testCommandHandler)
	require.NoError(t, err, "Первая регистрация обработчика не должна вызывать ошибку")

	// Повторно регистрируем обработчик.
	err = dispatcher.Register(testCommandHandler)

	// Проверяем ошибку.
	require.Error(t, err, "Повторная регистрация обработчика должна вызывать ошибку")
	assert.Contains(t, err.Error(), "обработчик для команды", "Текст ошибки должен содержать информацию о команде")
	assert.Contains(t, err.Error(), "уже зарегистрирован", "Текст ошибки должен содержать информацию о том, что обработчик уже зарегистрирован")
}

// Тест успешного получения диспетчера из реестра.
func TestRegistry_GetDispatcher_Success(t *testing.T) {
	t.Parallel()

	registry := command.NewRegistry()
	commandName := "test.command"

	// Получаем диспетчер в первый раз.
	dispatcher1, err := command.Dispatcher[testCommand, string](registry, commandName)
	require.NoError(t, err, "Первое получение диспетчера не должно вызывать ошибку")
	require.NotNil(t, dispatcher1, "Диспетчер не должен быть nil")

	// Получаем диспетчер во второй раз.
	dispatcher2, err := command.Dispatcher[testCommand, string](registry, commandName)
	require.NoError(t, err, "Второе получение диспетчера не должно вызывать ошибку")
	require.NotNil(t, dispatcher2, "Диспетчер не должен быть nil")

	// Проверяем, что это один и тот же экземпляр.
	assert.Same(t, dispatcher1, dispatcher2, "Реестр должен возвращать один и тот же экземпляр диспетчера для одного имени")
}

// Тест ошибки при несовпадении типов в реестре.
func TestRegistry_GetDispatcher_TypeMismatch(t *testing.T) {
	t.Parallel()

	registry := command.NewRegistry()
	commandName := "test.command"

	// Регистрируем диспетчер с одним типом.
	_, err := command.Dispatcher[testCommand, string](registry, commandName)
	require.NoError(t, err, "Регистрация первого диспетчера не должна вызывать ошибку")

	// Пытаемся получить диспетчер с другим типом.
	_, err = command.Dispatcher[anotherTestCommand, int](registry, commandName)

	// Проверяем ошибку.
	require.Error(t, err, "Получение диспетчера с другим типом должно вызывать ошибку")
	assert.Equal(t, fmt.Sprintf("диспетчер для команды '%s' уже существует с другим типом", commandName), err.Error())
}

// Тест на потокобезопасность реестра.
func TestRegistry_GetDispatcher_Concurrency(t *testing.T) {
	t.Parallel()

	registry := command.NewRegistry()
	commandName := "concurrent.command"
	goroutines := 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Массив для хранения полученных диспетчеров.
	dispatchers := make([]command.IDispatcher[testCommand, string], goroutines)

	// Запускаем множество горутин для одновременного получения диспетчера.
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			dispatcher, err := command.Dispatcher[testCommand, string](registry, commandName)
			// Внутри горутины используем require, чтобы немедленно остановить ее в случае ошибки.
			require.NoError(t, err)
			require.NotNil(t, dispatcher)
			dispatchers[i] = dispatcher
		}(i)
	}

	wg.Wait()

	// Проверяем, что все горутины получили один и тот же экземпляр диспетчера.
	firstDispatcher := dispatchers[0]
	for i := 1; i < goroutines; i++ {
		assert.Same(t, firstDispatcher, dispatchers[i], "Все горутины должны получать один и тот же экземпляр диспетчера")
	}
}

// Тест для проверки корректной работы middleware с использованием функциональных опций.
func TestDispatcher_WithMiddleware(t *testing.T) {
	t.Parallel()

	// 1. Настройка
	executionOrder := make([]string, 0)
	var mu sync.Mutex

	// Middleware 1: Логирование начала и конца.
	m1 := func(next command.CommandHandler[testCommand, string]) command.CommandHandler[testCommand, string] {
		return func(ctx context.Context, cmd testCommand) (string, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "m1-start")
			mu.Unlock()

			res, err := next(ctx, cmd)

			mu.Lock()
			executionOrder = append(executionOrder, "m1-end")
			mu.Unlock()
			return "m1-res-" + res, err
		}
	}

	// Middleware 2: Добавление значения в контекст.
	type key string
	const contextKey key = "middleware-key"
	m2 := func(next command.CommandHandler[testCommand, string]) command.CommandHandler[testCommand, string] {
		return func(ctx context.Context, cmd testCommand) (string, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "m2-start")
			mu.Unlock()

			ctx = context.WithValue(ctx, contextKey, "m2-value")
			res, err := next(ctx, cmd)

			mu.Lock()
			executionOrder = append(executionOrder, "m2-end")
			mu.Unlock()
			return "m2-res-" + res, err
		}
	}

	// Создаем диспетчер с middleware
	dispatcher := command.NewDispatcher[testCommand, string](
		command.WithMiddleware(m1),
		command.WithMiddleware(m2),
	)

	// Основной обработчик, который проверяет значение из контекста
	handler := func(ctx context.Context, cmd testCommand) (string, error) {
		mu.Lock()
		executionOrder = append(executionOrder, "handler")
		mu.Unlock()

		val, ok := ctx.Value(contextKey).(string)
		if !ok || val != "m2-value" {
			return "", fmt.Errorf("значение из контекста не найдено или некорректно")
		}
		return "processed: " + cmd.Value, nil
	}

	// 2. Регистрация
	err := dispatcher.Register(handler)
	require.NoError(t, err)

	// 3. Выполнение
	cmd := testCommand{Value: "test"}
	result, err := dispatcher.Dispatch(context.Background(), cmd)

	// 4. Проверка
	require.NoError(t, err)

	// Проверяем результат: middlewares оборачивают результат в обратном порядке
	assert.Equal(t, "m1-res-m2-res-processed: test", result)

	// Проверяем порядок выполнения: m1 -> m2 -> handler -> m2 -> m1
	expectedOrder := []string{"m1-start", "m2-start", "handler", "m2-end", "m1-end"}
	assert.Equal(t, expectedOrder, executionOrder)
}
