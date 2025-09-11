package query_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/x-research-team/dtx-framework/bus/query"
)

// Тестовый запрос для проверки.
type testQuery struct {
	Value string
}

// Тестовый запрос для проверки несовпадения типов.
type anotherTestQuery struct {
	Value int
}

// Тестовый обработчик запроса.
func testQueryHandler(ctx context.Context, q testQuery) (string, error) {
	return "processed: " + q.Value, nil
}

// Тест успешной регистрации и выполнения запроса.
func TestDispatcher_Success(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер.
	dispatcher := query.NewDispatcher[testQuery, string]()
	err := dispatcher.Register(testQueryHandler)
	require.NoError(t, err, "Регистрация обработчика не должна вызывать ошибку")

	// Отправляем запрос.
	q := testQuery{Value: "test"}
	result, err := dispatcher.Dispatch(context.Background(), q)

	// Проверяем результат.
	require.NoError(t, err, "Выполнение запроса не должно вызывать ошибку")
	assert.Equal(t, "processed: test", result, "Результат выполнения запроса некорректен")
}

// Тест ошибки при отправке запроса без зарегистрированного обработчика.
func TestDispatcher_Dispatch_NoHandler(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер без регистрации обработчика.
	dispatcher := query.NewDispatcher[testQuery, string]()

	// Отправляем запрос.
	q := testQuery{Value: "test"}
	_, err := dispatcher.Dispatch(context.Background(), q)

	// Проверяем ошибку.
	require.Error(t, err, "Выполнение запроса без обработчика должно вызывать ошибку")
	assert.Contains(t, err.Error(), "обработчик для запроса", "Текст ошибки должен содержать информацию об отсутствующем обработчике")
	assert.Contains(t, err.Error(), "не найден", "Текст ошибки должен содержать информацию о том, что обработчик не найден")
}

// Тест ошибки при повторной регистрации обработчика.
func TestDispatcher_Register_AlreadyRegistered(t *testing.T) {
	t.Parallel()

	// Создаем новый диспетчер и регистрируем обработчик.
	dispatcher := query.NewDispatcher[testQuery, string]()
	err := dispatcher.Register(testQueryHandler)
	require.NoError(t, err, "Первая регистрация обработчика не должна вызывать ошибку")

	// Повторно регистрируем обработчик.
	err = dispatcher.Register(testQueryHandler)

	// Проверяем ошибку.
	require.Error(t, err, "Повторная регистрация обработчика должна вызывать ошибку")
	assert.Contains(t, err.Error(), "обработчик для запроса", "Текст ошибки должен содержать информацию о запросе")
	assert.Contains(t, err.Error(), "уже зарегистрирован", "Текст ошибки должен содержать информацию о том, что обработчик уже зарегистрирован")
}

// Тест успешного получения диспетчера из реестра.
func TestRegistry_GetDispatcher_Success(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	queryName := "test.query"

	// Получаем диспетчер в первый раз.
	dispatcher1, err := query.Dispatcher[testQuery, string](registry, queryName)
	require.NoError(t, err, "Первое получение диспетчера не должно вызывать ошибку")
	require.NotNil(t, dispatcher1, "Диспетчер не должен быть nil")

	// Получаем диспетчер во второй раз.
	dispatcher2, err := query.Dispatcher[testQuery, string](registry, queryName)
	require.NoError(t, err, "Второе получение диспетчера не должно вызывать ошибку")
	require.NotNil(t, dispatcher2, "Диспетчер не должен быть nil")

	// Проверяем, что это один и тот же экземпляр.
	assert.Same(t, dispatcher1, dispatcher2, "Реестр должен возвращать один и тот же экземпляр диспетчера для одного имени")
}

// Тест ошибки при несовпадении типов в реестре.
func TestRegistry_GetDispatcher_TypeMismatch(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	queryName := "test.query"

	// Регистрируем диспетчер с одним типом.
	_, err := query.Dispatcher[testQuery, string](registry, queryName)
	require.NoError(t, err, "Регистрация первого диспетчера не должна вызывать ошибку")

	// Пытаемся получить диспетчер с другим типом.
	_, err = query.Dispatcher[anotherTestQuery, int](registry, queryName)

	// Проверяем ошибку.
	require.Error(t, err, "Получение диспетчера с другим типом должно вызывать ошибку")
	assert.Equal(t, fmt.Sprintf("диспетчер для запроса '%s' уже существует с другим типом", queryName), err.Error())
}

// Тест на потокобезопасность реестра.
func TestRegistry_GetDispatcher_Concurrency(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	queryName := "concurrent.query"
	goroutines := 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Массив для хранения полученных диспетчеров.
	dispatchers := make([]query.IDispatcher[testQuery, string], goroutines)

	// Запускаем множество горутин для одновременного получения диспетчера.
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			dispatcher, err := query.Dispatcher[testQuery, string](registry, queryName)
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

// Тест для проверки корректной работы middleware.
func TestDispatcher_WithMiddleware(t *testing.T) {
	t.Parallel()

	type contextKey string
	const (
		key1 = contextKey("key1")
		key2 = contextKey("key2")
	)

	// Middleware 1: добавляет значение в контекст и к результату.
	mw1 := func(next query.QueryHandler[testQuery, string]) query.QueryHandler[testQuery, string] {
		return func(ctx context.Context, q testQuery) (string, error) {
			ctx = context.WithValue(ctx, key1, "value1")
			res, err := next(ctx, q)
			return "mw1-" + res, err
		}
	}

	// Middleware 2: добавляет значение в контекст и к результату, проверяет значение из mw1.
	mw2 := func(next query.QueryHandler[testQuery, string]) query.QueryHandler[testQuery, string] {
		return func(ctx context.Context, q testQuery) (string, error) {
			val, ok := ctx.Value(key1).(string)
			require.True(t, ok, "Значение из mw1 должно быть в контексте")
			assert.Equal(t, "value1", val, "Значение из mw1 некорректно")

			ctx = context.WithValue(ctx, key2, "value2")
			res, err := next(ctx, q)
			return "mw2-" + res, err
		}
	}

	// Обработчик, который проверяет значения из обоих middlewares.
	handler := func(ctx context.Context, q testQuery) (string, error) {
		val1, ok1 := ctx.Value(key1).(string)
		require.True(t, ok1, "Значение из mw1 должно быть в контексте в обработчике")
		assert.Equal(t, "value1", val1, "Значение из mw1 в обработчике некорректно")

		val2, ok2 := ctx.Value(key2).(string)
		require.True(t, ok2, "Значение из mw2 должно быть в контексте в обработчике")
		assert.Equal(t, "value2", val2, "Значение из mw2 в обработчике некорректно")

		return "handler", nil
	}

	// Создаем диспетчер с middleware.
	dispatcher := query.NewDispatcher[testQuery, string](
		query.WithMiddleware[testQuery, string](mw1),
		query.WithMiddleware[testQuery, string](mw2),
	)
	err := dispatcher.Register(handler)
	require.NoError(t, err)

	// Выполняем запрос.
	result, err := dispatcher.Dispatch(context.Background(), testQuery{})
	require.NoError(t, err)

	// Проверяем, что все middlewares и обработчик были вызваны в правильном порядке.
	assert.Equal(t, "mw1-mw2-handler", result)
}