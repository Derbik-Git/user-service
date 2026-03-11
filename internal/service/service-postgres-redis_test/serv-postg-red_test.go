package servPostgRedTest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/stretchr/testify/require"
)

/*
✔ Create user
✔ Unique violation
✔ Cache miss
✔ Cache hit
✔ Delete consistency
✔ Update overwrite
✔ TTL refresh
✔ TTL expiration
✔ Consistency
✔ Concurrent access
✔ Concurrent update
✔ Redis fallback
*/

func newUniqueUser() *domain.User {
	id := time.Now().UnixNano()

	return &domain.User{
		ID:        id,
		Email:     fmt.Sprintf("test+%d@gmail.com", id),
		Name:      "ItSvcPostRedTest",
		CreatedAt: time.Now(),
	}
}

//Эта функция показатель того, как не нужно делать при паралельный тестах, так как постоянно данные будут скидываться и тесты будут фапится, то есть при неизменном коде будут разные результаты, то всё нормально, то в следующем запуске ошибка, то данные не корректные и так далее
/*
func flushRedis(t *testing.T) { // чистим редис после каждого теста
	require.NoError(t, env.Cache.Client().FlushDB(context.Background()).Err()) // FlushDB удаляет все ключи текущей базы данных.
}
*/

// Простейшая проверка на создание пользователя
func TestService_CreateUser(t *testing.T) {
	t.Parallel()

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)

	getUser, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	require.Equal(t, user.Email, getUser.Email)
}

// Тут вообще должен был быть тест на то, что если в постгресе вернулась ошибка о уникальности, то редис не должен был записать, но так как у меня метод создания пользователя в сервисе не добавляет пользователя в редис, а только создаёт его в постгрес, то редис и не будет ничего записывать, так что я просто проверю что при создании пользователя с таким же email, постгрес вернёт ошибку, а редис не запишет
func TestService_Create_UniqueViolation(t *testing.T) {
	t.Parallel()

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)

	_, err = env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.Error(t, err)
}

func TestService_CacheMiss_ToRedis(t *testing.T) { // проверяем как программа кладёт пользователя в кеш, если его там нет, из названия следует: "тест сервиса если в кеше ничего нет", собственно поэтому мы в конце проверяем на NotNil так как мы ожидаем что пользователь в кеше благополучно создаться
	t.Parallel()

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

	ctx := context.Background()

	user := newUniqueUser()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	require.NoError(t, env.Svc.DeleteUser(ctx, user.ID))

	_, err = env.Repo.GetUserByID(ctx, user.ID)
	require.NoError(t, err) // у нас код под капотом возвращает nil, nil, и это не будет являтся ошибкой, поэтому тест продолжит выполнение

	_, err = env.Cache.GetUser(ctx, user.ID)
	require.NoError(t, err) // туту аналогично как и с репозиторием в строке 88
}

func TestService_Update_OverwriteCache(t *testing.T) { // проверяем благополучно ли сохраняются значения и в pg и в redis
	t.Parallel()

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

func TestService_CacheMiss(t *testing.T) { // это очень интересный тест, потому что если с редисом нет соединения, то просто блок с редисом не будет выполнятся, но вот если соединение есть и пользователь был как бы добавлен в pg и сделан get запрос соответственно он был добавлен и в редис, но вдруг мы его от туда удаляем и у нас интерес, выдаст ли программа nil, nil или же как нужно достанет пользователя из pg
	t.Parallel()

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

// Проверяет устойчивость сервиса к ошибке подключения к Redis: если Redis недоступен, сервис должен корректно работать только с Postgres.
func TestService_RedisFallbackToPG(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	user := newUniqueUser()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Redis с несуществующим адресом
	// Создаём "сломанную" реализацию Redis: этот тест проверяет устойчивость к недоступности Redis и корректную работу сервиса только с Postgres, если Redis недоступен (например, по несуществующему адресу).
	brokenCache, err := cache.NewRedisCache(
		[]string{"localhost:6333"}, // !!!!! несуществующий порт, важно сделать именно с портом, потому что если сделаем пустым значением, то тест упадёт, но если укажем несуществующий порт и уберём PING(он не даст запустится с несуществующим портом) в NewRedisCache и будем пинговать только в main.go, то тогда fallback будет нормально работать без redis, потому что такого порта нет и сервисный слой проигнорурует cache с помощью блока if и пойдёт в pg, ошибка будет только тогда, когда с несуществующим портом вызовется команда redis.Cmdable, но этого не произойдёт за счёт блока if в сервисе, блок просто проигнорирует redis
		5*time.Second,
		nil,
		logger,
	)
	require.NoError(t, err)

	svc := service.NewUserService(env.Repo, brokenCache, logger, 5*time.Second)

	result, err := svc.GetUser(ctx, user.ID)

	require.NoError(t, err)
	require.Equal(t, user.Email, result.Email)
}

func TestService_TTL_SetAndFresh(t *testing.T) { // проверяем ttl, нормлаьно ли обновляется
	t.Parallel()

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
	require.Greater(t, ttl2.Seconds(), 0.0) // Проверяем, что TTL после обновления снова положительный
}

// Проверка потокобезопасности
func TestService_ConcurrencyAccess(t *testing.T) {
	t.Parallel()

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	var wg sync.WaitGroup

	for i := 0; i <= 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := env.Svc.GetUser(ctx, user.ID) // В РАБОТЕ С ГОРУТИНАМИ РАБОТАТАТЬ С require.NoError КОНКУРЕНТНО НЕ БЕЗОПАСНО ИСПОЛЬЗОВАТЬ ВНУТРИ ГОРУТИНЫ, так как из за этого завершится целый тест а не горутина
			if err != nil {
				t.Errorf("test concurency get failed : %v", err)
			}
		}()
	}
	wg.Wait()
}

// Если TTL истёк -> следующий get идёт в Postgres и заного кладёт в Redis
func TestService_TTL_Expiration(t *testing.T) {
	t.Parallel()

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	time.Sleep(6 * time.Second)

	// Ожидаем пустой redis, так как TTL истёк
	_, err = env.Cache.GetUser(ctx, user.ID)
	require.Error(t, err)

	// это что бы значение заного добавилось в redis
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	CacheGetTrue, err := env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, user.Email, CacheGetTrue.Email)
}

// Проверка race condition при одновременном обновлении и получении данных
func TestService_Concurrent_UpdateAndGet(t *testing.T) {
	t.Parallel()

	user := newUniqueUser()
	ctx := context.Background()

	_, err := env.Svc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)
	_, err = env.Svc.GetUser(ctx, user.ID)
	require.NoError(t, err)

	var wg sync.WaitGroup

	for i := 0; i <= 20; i++ {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()

			u := *user // !!!! Если внутри горутины ты бы просто писал user.Email = …, то все 50+ потоков работали бы с одной и той же структурой, т.е. гонка данных гарантирована. Любая запись могла бы «перебить» другая, а проверка результатов в конце оказалась бы бессмысленной. Поэтому мы создаём копию структуры user для каждой горутины, и уже с ней работаем. Таким образом, каждая горутина работает со своей собственной копией данных, и гонки данных не возникает.
			u.Email = fmt.Sprintf("updated-%d@test.com", i)
			_, _ = env.Svc.UpdateUser(ctx, &u)
		}(i)

		go func() {
			defer wg.Done()
			_, _ = env.Svc.GetUser(ctx, user.ID)
		}()
	}

	wg.Wait()

	// В конце данные должны быть консистентны
	pgUser, err := env.Repo.GetUserByID(ctx, user.ID)
	require.NoError(t, err)

	cacheUser, err := env.Cache.GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, pgUser.Email, cacheUser.Email)
}
