package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *RedisCache {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cache, err := NewRedisCache([]string{"localhost:6379"}, 3*time.Second, nil, logger)
	require.NoError(t, err)

	t.Cleanup(func() { // регестрируем закрытие клиента после теста
		_ = cache.Close()
	})

	return cache
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

	user := newUniqueUser(1)

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

	user := newUniqueUser(2)

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

	user := newUniqueUser(3)

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

	user := newUniqueUser(4)

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

			user := newUniqueUser(int64(i))
			require.NoError(t, cache.SetUser(ctx, user, 0))

			_, err := cache.GetUser(ctx, user.ID)
			require.NoError(t, err)
		}(int64(i))
	}

	wg.Wait()
}

func TestRedis_DeleteNonExistentUser(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	err := cache.DeleteUser(ctx, 88888888)
	require.NoError(t, err)
	assert.Contains(t, err, "redis DEL failed")
}

func TestRedis_GetNonKeyRedis(t *testing.T) { // пытаемся достать пользователя из несуществуюшего ключа redis, поэтому ожидаем ошибку
	t.Parallel()

	cache := newTestCache(t)
	ctx := context.Background()

	_, err := cache.GetUser(ctx, 7777777)
	require.NoError(t, err)
	assert.Contains(t, err, "redis GET failed")
}

// set с nil не делаем, потому что этого не допускает логика слоя cache
