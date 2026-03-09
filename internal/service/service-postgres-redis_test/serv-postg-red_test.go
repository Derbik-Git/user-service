package servPostgRedTest

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/stretchr/testify/require"
)

func newUniqueUser() *domain.User {
	id := time.Now().UnixNano()
	return &domain.User{
		ID:        id,
		Email:     fmt.Sprintf("test+%d@gmail", id),
		Name:      "ItSvcPostRedTest",
		CreatedAt: time.Now(),
	}
}

func flushRedis(t *testing.T) { // чистим редис после каждого теста
	require.NoError(t, env.Cache.Client().FlushDB(context.Background()).Err()) // FlushDB удаляет все ключи текущей базы данных.
}

func TestService_CacheMiss_ToRedis(t *testing.T) { // проверяем как программа кладёт пользователя в кеш, если его там нет, из названия следует: "тест сервиса если в кеше ничего нет", собственно поэтому мы в конце проверяем на NotNil так как мы ожидаем что пользователь в кеше благополучно создаться
	t.Parallel()

	flushRedis(t)
	ctx := context.Background()

	user := newUniqueUser()

	_, err := env.Svc.CreateUser(ctx, user.Name, user.Email)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID) // это нужно для того что бы пользователь добавился и в редис потому что под капотом(в сервисе) этот метод добавляет пользователя в redis, а create не добавляет
	require.NoError(t, err)

	resultPgGet, err := env.Repo.GetUserByID(ctx, user.ID) // под капотом в сервисе за счёт этого запроса пользователь при гет запросе в постгрес пользователь добавляется в redis
	require.NoError(t, err)
	require.Equal(t, user.Email, resultPgGet.Email)

	resultCacheGet, err := env.Cache.GetUser(ctx, user.ID) // проверяем как метод гет из сервиса добавил пользователя в редиc
	require.NoError(t, err)
	require.NotNil(t, resultCacheGet)
}

func TestService_CacheHit(t *testing.T) { //Удаляем из постгрес и достаём из редис
	t.Parallel()

	flushRedis(t)
	ctx := context.Background()

	user := newUniqueUser()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID) // нужно для добавления пользователя в redis, createUser в сервисе не добавляет пользователя в redis, а создаёт его только в Postgres
	require.NoError(t, err)

	require.Error(t, env.Repo.Delete(ctx, user.ID))

	cacheGetResult, err := env.Svc.GetUser(ctx, user.ID) // так как из pg удалено ожидаем что вернёт redis
	require.NoError(t, err)
	require.Equal(t, user.Email, cacheGetResult.Email)
}

func TestService_Delete_Consistency(t *testing.T) { // удаляем сервисом, смотрим как удалилось и там и там
	t.Parallel()

	flushRedis(t)
	ctx := context.Background()

	user := newUniqueUser()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	require.NoError(t, env.Svc.DeleteUser(ctx, user.ID))

	_, err = env.Repo.GetUserByID(ctx, user.ID)
	require.NoError(t, err)

	_, err = env.Cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
}

func TestService_Update_OverwriteCache(t *testing.T) { // проверяем благополучно ли сохраняются значения и в pg и в redis
	t.Parallel()

	flushRedis(t)

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	user.Email = "update@email"

	_, err = env.Svc.UpdateUser(ctx, user)
	require.NoError(t, err)

	repoUpdateResult, err := env.Repo.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "update@email", repoUpdateResult.Email)

	cacheUpdateResult, err := env.Cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "update@email", cacheUpdateResult.Email)
}

func TestService_FullConsistency(t *testing.T) { // проверяем все ли между собой благополучно работает, и сервис и pg и redis
	t.Parallel()

	flushRedis(t)

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	serviceUser, _ := env.Svc.GetUser(ctx, user.ID)
	postgresUser, _ := env.Repo.GetUserByID(ctx, user.ID)
	cacheUser, _ := env.Cache.GetUser(ctx, user.ID)

	require.Equal(t, serviceUser.Email, postgresUser.Email)
	require.Equal(t, postgresUser.Email, cacheUser.Email)
}

func TestService_RedisFallbackToPG(t *testing.T) { // это очень интересный тест, потому что если с редисом нет соединения, то просто блок с редисом не будет выполнятся, но вот если соединение есть и пользователь был как бы добавлен в pg и сделан get запрос соответственно он был добавлен и в редис, но вдруг мы его от туда удаляем и у нас интерес, выдаст ли программа nil, nil или же как нужно достанет пользователя из pg
	t.Parallel()

	flushRedis(t)

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	require.NoError(t, env.Cache.DeleteUser(ctx, user.ID))

	getPG, err := env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, user.Email, getPG.Email)
}

func TestService_TTL_SetAndFresh(t *testing.T) { // проверяем ttl, нормлаьно ли обновляется
	t.Parallel()

	flushRedis(t)
	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	key := "user:" + strconv.Itoa(int(user.ID)) // нужно лишь для функции TTL что бы достать значение токена

	ttl1, err := env.Cache.Client().TTL(ctx, key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl1.Seconds(), 0.0) // Утверждаем, что значение, возвращаемое ttl1.Seconds(), больше 0.0. Если это не так, то тест немедленно завершается с ошибкой, и в сообщении об ошибке будет указано, что утверждение не выполнено.

	_, err = env.Svc.UpdateUser(ctx, user)
	require.NoError(t, err)

	ttl2, err := env.Cache.Client().TTL(ctx, key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl2, ttl1)
}

func TestService_Concurrent

// убедится что если постгрес вренул ошибку о создании, редис не записал, то есть если постгрес вернул ошибку о уникальности например, то редис не должен это записать, убедится самому(не за счёт тестов что у меня в коде норм всё будет работать, с gpt глазами посмотреть и поспрашивать а норм ли, это скорее вего будет в сервисной логике в get/set)
// потом написать самый простейший тест нормально ли при create создаётся пользователь, и нужно поставить его в начало
// ещё попробуй просто для красоты и читаемости кода поставить тесты попроще в начале, а потом переходить к сложному
// убедитмя что TestService_RedisFallbackToPG будет выполнятся коректно, но вообще должен
