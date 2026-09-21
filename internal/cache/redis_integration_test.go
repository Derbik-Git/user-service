package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *RedisCache {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	addrs := []string{
		"localhost:7001", "localhost:7002", "localhost:7003",
		"localhost:7004", "localhost:7005", "localhost:7006",
	}

	cache, err := NewRedisCache(addrs, 3*time.Second, nil, logger)
	require.NoError(t, err)

	t.Cleanup(func() { // регестрируем закрытие клиента после теста
		_ = cache.Close()
	})

	return cache
}

var idSec int64 = 100

func getNextTestID() int64 {
	return atomic.AddInt64(&idSec, 1)
}

func newUniqueUser(idUniqueRedisKey int64) *domain.User { // этот метод с уникальным пользователем(добавлено из за t.Parallel() в тесте) для того, что бы при параллельном выполнении тестов, каждый тест работал со своей структурой пользователя, а не с общей, так как если будет общая структура, то тесты будут мешать друг другу и могут падать, например если один тест удалит пользователя, а другой тест будет пытаться его достать, то он упадёт, так как пользователь уже удалён, а так как мы добавляем суффикс к email, то каждый тест будет работать со своим пользователем и не будет мешать другим тестам
	return &domain.User{
		ID:        idUniqueRedisKey,
		Email:     fmt.Sprintf("test+%d@gmail.com", idUniqueRedisKey),
		Name:      fmt.Sprintf("Johan%d", idUniqueRedisKey),
		CreatedAt: time.Unix(17000000000, 0),
	}
}

func TestRedis_SetAndGetUser(t *testing.T) {
	t.Parallel() // если ставим t.Parallel(), то везде должны быть уникальные ключи пользователя, что бы каждый тест выполняясь паралельно работал со своей структурой, иначе если
	cache := newTestCache(t)
	ctx := context.Background()

	user := newUniqueUser(getNextTestID())

	err := cache.SetUser(ctx, user, 0) // ttl = 0, берётся из RedisCache.ttl (а как от туда берётся и что это, смотри логику SetUser в логике кеша) В КРАТЦЕ в логике прописано if ttl <= 0 {tt = c.ttl}, где с - это структура RedisCache, а мы функцией NewRedisCache заполняем эту структуру, например как мы в методе выше передали этому значению 3 секунды, а точнее 3*time.Second
	require.NoError(t, err)

	result, err := cache.GetUser(ctx, user.ID) // & на user.ID не нужен, так как GetUser возвращает указатель на структуру User
	require.NoError(t, err)
	require.Equal(t, user, result)
}

func TestRedis_DeleteUser(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	user := newUniqueUser(getNextTestID())

	require.NoError(t, cache.SetUser(ctx, user, 0))
	require.NoError(t, cache.DeleteUser(ctx, user.ID))

	result, err := cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Nil(t, result) // так как мы удалили пользователя, то при попытке его достать, мы должны получить nil, так как его уже нет в кеше
}

func TestRedis_TTL(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	user := newUniqueUser(getNextTestID())

	require.NoError(t, cache.SetUser(ctx, user, 2*time.Second))

	time.Sleep(3 * time.Second) // ждём 3 секунды, что бы пользователь удалился из кеша, так как ttl 2 секунды

	result, err := cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Nil(t, result) // так как ttl 2 секунды, то после 3 секунд пользователь должен удалиться из кеша, и при попытке его достать, мы должны получить nil
}

func TestRedis_Overwrite(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	user := newUniqueUser(getNextTestID())

	require.NoError(t, cache.SetUser(ctx, user, 0))

	user.Email = "new-" + user.Email                // изменяем email, что бы проверить перезапись данных в кеше
	require.NoError(t, cache.SetUser(ctx, user, 0)) // повтороно вызывая этот метод с тем же ключом(ID), мы не добавляем новое поле, а перезаписываем старое с переданным ID

	result, err := cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, user.Email, result.Email)
}

func TestRedis_CacheMiss(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	result, err := cache.GetUser(ctx, 999999999) // пытаемся достать пользователя с несуществующим ID, что бы проверить поведение при промахе кеша
	require.NoError(t, err)
	require.Nil(t, result) // так как такого пользователя нет, то мы должны получить nil, а не ошибку, так как в логике кеша прописано if errors.Is(err, redis.Nil) {return nil, nil}, то есть если ключ существует, но значение пустое, то мы возвращаем nil, nil, что означает что всё ок, но значение по ключу пустое
}

func TestRedis_Concurency(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	const goroutins = 55
	wg := sync.WaitGroup{}

	for i := 5; i < goroutins; i++ { // 5 что бы не пересекаться с ID пользователей из других тестов, те что выше и не нарушать уникальность ключей redis
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()

			user := newUniqueUser(getNextTestID())

			// в документации require написано что категорически нельзя использовать t.FailNow() (который под капотом использует require) из дочерних горутин, так как он в случае ошибки положит основную горутину теста, поэтому мы используем assert
			if !assert.NoError(t, cache.SetUser(ctx, user, 0)) {
				return // если не получилось, закрываем только эту горутину
			}
			_, err := cache.GetUser(ctx, user.ID)
			assert.NoError(t, err)
		}(int64(i))
	}

	wg.Wait()
}

func TestRedis_DeleteNonExistentUser(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	err := cache.DeleteUser(ctx, 88888888)
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis DEL failed")
}

func TestRedis_GetNonKeyRedis(t *testing.T) { // пытаемся достать пользователя из несуществуюшего ключа redis, поэтому ожидаем ошибку
	t.Parallel()
	cache := newTestCache(t)

	// Создаем и сразу отменяем контекст
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cache.GetUser(ctx, 7777777)
	require.Error(t, err)

	// Меняем ожидаемый текст на "context canceled"
	assert.ErrorContains(t, err, "context canceled")
}

// set с nil не делаем, потому что этого не допускает логика слоя cache

func TestRedis_DeleteUser_RedisError(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()
	user := newUniqueUser(getNextTestID())

	require.NoError(t, cache.Close())

	err := cache.DeleteUser(ctx, user.ID)
	require.Error(t, err)

	assert.ErrorContains(t, err, "redis: client is closed")
}
