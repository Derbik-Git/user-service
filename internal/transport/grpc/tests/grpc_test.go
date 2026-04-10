package grpcTest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// проверяем не ломает ли запрос metadata в email(time.Now().UnixNano()) и не ломает ли он валидацию. Эта проверка нужна, так как наши тесты выполняются параллельно, и в случае, если у нас будет один и тот же email, тесты будут менять одно и то же поле, удалять однои тоже поле, куча раз его создавать, а это приведёт race condition, и бд будет возвращать ошибку об уникальностит, что такой email уже есть. Правда UnixNano не нужен если email разный, но я ставляю как практику, и как гарантию что имя будет уникальное со 100% вероятностью, и я понимаю что такую гарантию даёт не нано, а разные email, просто в будущем часто придётся использовать UnixNano, поэтому практикую его применение в разных ситуациях и практикую вариант, где email мог быть одинаковым
func TestGRPC_Metadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCMetadata_%d@gmail.com", time.Now().UnixNano()),
		Name:  "gRPCTest",
	})

	require.NoError(t, err)
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

	ctx := context.Background()

	create, _ := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCGet-%d@email.com", time.Now().UnixNano()),
		Name:  "gRPCTestGet",
	})

	resp, err := client.GetUser(ctx, &userv1.GetUserRequest{
		Id: create.User.Id,
	})

	require.NoError(t, err)
	require.Equal(t, create.User.Id, resp.User.Id)
}

func TestGRPC_NotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

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
}

func TestGRPC_Update(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	create, _ := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: fmt.Sprintf("testGRPCUpdate-%d@email.com", time.Now().UnixNano()),
		Name:  "gRPCTest",
	})

	updated, err := client.UpdateUser(ctx, &userv1.UpdateUserRequest{
		Id:    create.User.Id,
		Email: "updated@gmail.com",
		Name:  "testGRPCUpdated",
	})

	require.NoError(t, err)
	require.Equal(t, "updated@gmail.com", updated.User.Email)
	require.Equal(t, "testGRPCUpdated", updated.User.Name)
}

func TestGRPC_Concurrent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var wg sync.WaitGroup

	for i := 0; i <= 50; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
				Email: fmt.Sprintf("concurent-%d@gmail.com", i),
				Name:  fmt.Sprintf("testGRPCConcurent%d", i),
			})

			if err != nil {
				t.Errorf("concurent failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

// Проверяем выдаёт ли повторный одинаковый create, ОШИБКУ
func TestGRPC_Idempotency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	email := fmt.Sprintf("testGRPCCreate-%d@email.com", time.Now().UnixNano())

	_, _ = client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: email,
		Name:  "testGRPCIdempotency1",
	})

	_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
		Email: email,
		Name:  "testGRPCIdempotency2",
	})
	require.Error(t, err)
}

// Классический тест на нагрузку
func TestGRPC_Load(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for i := 1; i <= 200; i++ {
		_, err := client.CreateUser(ctx, &userv1.CreateUserRequest{
			Email: fmt.Sprintf("load-%d@gmail.com"),
			Name:  "testGRPCLoad",
		})
		require.NoError(t, err)
	}
}
