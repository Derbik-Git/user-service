package servPostgRedKafkaTest

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Проверяет устойчивость сервиса к ошибке подключения к Redis: если Redis недоступен, сервис должен корректно работать только с Postgres.
func TestService_RedisFallbackToPG(t *testing.T) {
	require.NotNil(t, env, "TestEnv не инициализирован в TestMain")
	require.NotNil(t, env.Svc, "env.Svc равен nil")

	// этот тест не должен выполнятся параллельно, так как из за несуществующего порта для тестов, редис пытается подключится множество раз, что видно по логам, это и душит общую среду выполнения(всё зависает), из-за чего kafka и не успевает получить ивент, за отведённые 5 секунд и падает по таймауту

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	user := newUniqueUser("TestService_RedisFallbackToPG", int64(uuid.New().ID()))

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Redis с несуществующим адресом
	// Создаём "сломанную" реализацию Redis: этот тест проверяет устойчивость к недоступности Redis и корректную работу сервиса только с Postgres, если Redis недоступен (например, по несуществующему адресу).
	brokenCache, err := cache.NewRedisCache(
		[]string{"127.0.0.1:6333"}, // !!!!! несуществующий порт, важно сделать именно с портом, потому что если сделаем пустым значением, то тест упадёт, но если укажем несуществующий порт и уберём PING(он не даст запустится с несуществующим портом) в NewRedisCache и будем пинговать только в main.go, то тогда fallback будет нормально работать без redis, потому что такого порта нет и сервисный слой проигнорурует cache с помощью блока if и пойдёт в pg, ошибка будет только тогда, когда с несуществующим портом вызовется команда redis.Cmdable, но этого не произойдёт за счёт блока if в сервисе, блок просто проигнорирует redis
		5*time.Second,
		nil,
		logger,
	)
	require.NoError(t, err)
	defer brokenCache.Close()

	LocalSvc := service.NewUserService(env.Repo, brokenCache, env.KafkaProducer, logger, 5*time.Second)

	createdUser, err := LocalSvc.CreateUser(ctx, user.Email, user.Name)
	require.NoError(t, err)

	result, err := LocalSvc.GetUser(ctx, createdUser.ID)
	require.NoError(t, err)
	require.Equal(t, createdUser.Email, result.Email)
}
