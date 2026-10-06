package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// тут мы проваеряем не ломает ли какая то доп информация(metadata) сам запрос, проверка на токены или же id, для отслежки запроса в других микросервисов, всё чт освязано с metadata
func TestGRPC_Metadata(t *testing.T) {
	t.Parallel()

	// Используем валидный контекст с токеном и добавляем туда доп. заголовки
	ctx := authCtx(100)
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "test-req-12345")

	resp, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCMetadata_%d@gmail.com", time.Now().UnixNano()),
		Name:  "gRPCTest",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.User)
}

func TestGRPC_CreateUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCCreate-%d@email.com", time.Now().UnixNano()),
		Name:  "gRPCTestCreate",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.User)
}

func TestGRPC_GetUser(t *testing.T) {
	t.Parallel()

	create, err := client.CreateUser(context.Background(), &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCGet-%d@email.com", time.Now().UnixNano()),
		Name:  "gRPCTestGet",
	})

	// Делаем запрос GetUser с ТОКЕНОМ, выданным на ID созданного юзера
	ctx := authCtx(create.User.Id)

	resp, err := client.GetUser(ctx, &userv1.GetUserRequest{
		Id: create.User.Id,
	})

	require.NoError(t, err)
	require.Equal(t, create.User.Id, resp.User.Id)
}

func TestGRPC_NotFound(t *testing.T) {
	t.Parallel()

	nonExistentID := int64(99999)
	// Передаем токен с этим ID, чтобы пройти авторизацию и дойти до уровня БД
	ctx := authCtx(nonExistentID)

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{
		Id: 99999,
	})

	st, _ := status.FromError(err)              //  Эта функция преобразует стандартную ошибку Go в объект status, который содержит код gRPC-ошибки и сообщение.
	require.Equal(t, codes.NotFound, st.Code()) // !! проверка того, что код ошибки, возвращённый сервером, действительно равен codes.NotFound. Это гарантирует, что сервер ведёт себя ожидаемо при запросе отсутствующего ресурса.
}

func TestGRPC_Validation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: "",
		Name:  "",
	})

	st, _ := status.FromError(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGRPC_Dublication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	email := fmt.Sprintf("testGRPCDublication-%d@email.com", time.Now().UnixNano())

	_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: email,
		Name:  "testGRPCDublication",
	})
	require.NoError(t, err)

	_, err = client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: email,
		Name:  "testGRPCDublication",
	})

	st, _ := status.FromError(err)
	require.Equal(t, codes.AlreadyExists, st.Code())

}

// этот тест проверяет что происходит в случае, если контекст завершается раньше, чем мы получили ответ
func TestGRPC_Deadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{
		Id: 1,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Expected gRPC error")

	// Проверяем, что код ошибки — это именно Таймаут (DeadlineExceeded) или Отмена (Canceled)
	// (Иногда gRPC клиент может отбить запрос со статусом Canceled, если контекст умер до отправки)
	require.Contains(t, []codes.Code{codes.DeadlineExceeded, codes.Canceled}, st.Code())
}

func TestGRPC_Update(t *testing.T) {
	t.Parallel()

	create, err := client.CreateUser(context.Background(), &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCUpdate-%d@email.com", time.Now().UnixNano()),
		Name:  "gRPCTest",
	})
	require.NoError(t, err, "failed to create user for update test")

	// Передаем контекст с токеном созданного пользователя
	ctx := authCtx(create.User.Id)
	updateEmail := fmt.Sprintf("updated-%d@gmail.com", time.Now().UnixNano())

	updated, err := client.UpdateUser(ctx, &userv1.UpdateUserRequest{
		Id:    create.User.Id,
		Email: updateEmail,
		Name:  "testGRPCUpdated",
	})

	require.NoError(t, err)
	require.Equal(t, updateEmail, updated.User.Email)
	require.Equal(t, "testGRPCUpdated", updated.User.Name)
}

func TestGRPC_Concurrent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var wg sync.WaitGroup

	// выносим inix.Nano из горутины, так как в ней, может создаться идентичное время из за многопоточности
	runID := time.Now().UnixNano()

	for i := 0; i <= 50; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			email := fmt.Sprintf("concurrent-%d-%d@gmail.com", runID, i)

			_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
				Email: email,
				Name:  fmt.Sprintf("testGRPCConcurent%d", i),
			})

			if err != nil {
				t.Errorf("concurent failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

// Классический тест на нагрузку
func TestGRPC_Load(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for i := 1; i <= 200; i++ {
		_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
			Email: fmt.Sprintf("load-%d-%d@gmail.com", i, time.Now().UnixNano()),
			Name:  fmt.Sprintf("testGRPCLoad%d", i),
		})
		require.NoError(t, err)
	}
}

// Тесты jwt:

// 1. Проверка: Запрос к защищенной ручке без токена возвращает Unauthenticated
func TestGRPC_Auth_MissingToken(t *testing.T) {
	t.Parallel()

	_, err := client.GetUser(context.Background(), &userv1.GetUserRequest{Id: 1})

	st, _ := status.FromError(err)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

// 2. Проверка: Пользователь А пытающийся получить профиль пользователя Б получает PermissionDenied
func TestGRPC_Auth_ForbiddenAccess(t *testing.T) {
	t.Parallel()

	// Авторизованы как юзер 10, но просим профиль юзера 999
	ctx := authCtx(10)

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 999})

	st, _ := status.FromError(err)
	require.Equal(t, codes.PermissionDenied, st.Code())
}
