package servPostgRedTest

import (
	"errors"
	"log"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/migrate"
	"github.com/Derbik-Git/user-service/internal/repository/postgres"
	"github.com/Derbik-Git/user-service/internal/service"
)

const (
	testPostgresDSNEnv  = "SERVICE_TEST_POSTGRES_DSN"
	testRedisAddrEnv    = "SERVICE_TESTREDIS_ADDR"
	testKafkaBrokersEnv = "SERVICE_TEST_KAFKA_BROKERS"
)

type TestEnv struct {
	Repo          *postgres.Storage
	Cache         *cache.RedisCache
	KafkaProducer *kafka.Producer
	KafkaConsumer *kafka.Consumer
	Svc           *service.Service
}

var env *TestEnv

// для запуска PostgreSQL требуется 2-5 секунд, в то время как наше приложение обращается к нему сразу(в случае если бд не успела открыться, приложение сразу будет падать, поэтому нам и нужна эта функция для ожидания полного подключения бд)
func waitForPostgres(dsn string) (*postgres.Storage, error) { // возвращаем структуру с соединением у бд, после ожидания её полной готовности
	for i := 0; i < 10; i++ {
		repo, err := postgres.NewStorage(dsn)
		if err == nil {
			return repo, nil
		}

		time.Sleep(time.Second)
	}

	return nil, errors.New("postgres not ready after retries")

}

func waitForKafka(brokers []string) error {
	for i := 0; i <= 15; i++ {
		for _, broker := range brokers {
			conn, err := net.DialTimeout("tcp", broker, time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("kafka not ready after retries")
}

func TestMain(m *testing.M) {
	dsn := os.Getenv(testPostgresDSNEnv)
	if dsn == "" {
		log.Fatalf("%s not set", testPostgresDSNEnv)
	}

	redisAddr := os.Getenv(testRedisAddrEnv)
	if redisAddr == "" {
		log.Fatalf("%s not set", testRedisAddrEnv)
	}

	testKafkaBrokers := os.Getenv(testKafkaBrokersEnv)
	testBrokers := []string{}
	if testKafkaBrokers != "" {
		testBrokers = []string{testKafkaBrokers}
	}

	if err := waitForKafka(testBrokers); err != nil {
		log.Fatal(err)

	}

	repo, err := waitForPostgres(dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := migrate.MigrateUp(repo.DB(), "../../migrations"); err != nil { // переадём соединение с бд для запуска миграций, функция DB тупо возвращает уже созданное соединение с бд созданное методом waitForPostgres
		log.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cache, err := cache.NewRedisCache([]string{redisAddr}, 5*time.Second, nil, logger)
	if err != nil {
		log.Fatal(err)
	}

	testTopic := "test-topic-user-events"
	testGroupID := "test-groupID"

	producer := kafka.NewProducer(testBrokers)
	consumer := kafka.NewConsumer(testBrokers, testTopic, testGroupID, logger)

	env = &TestEnv{
		Repo:  repo,
		Cache: cache,
		Svc:   service.NewUserService(repo, cache, producer, logger, 5*time.Second),
	}

	code := m.Run()

	_ = env.Cache.Close()
	_ = env.Repo.Close()

	os.Exit(code)
}
