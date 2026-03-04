package servPostgRedTest

import (
	"context"
	"fmt"
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

// убедится что если постгрес вренул ошибку о создании, редис не записал, то есть если постгрес вернул ошибку о уникальности например, то редис не должен это записать, убедится самому(не за счёт тестов что у меня в коде норм всё будет работать, с gpt глазами посмотреть и поспрашивать а норм ли, это скорее вего будет в сервисной логике в get/set)
// потом написать самый простейший тест нормально ли при create создаётся пользователь, и нужно поставить его в начало
