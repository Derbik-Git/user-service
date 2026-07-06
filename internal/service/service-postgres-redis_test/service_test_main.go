package servPostgRedKafkaTest

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/domain"
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

// Корзина для перехвата сообщений для тестов из consumer за счёт метода Add, который берёт event из консьюмера и кладёт его в эту структуру, из которой мы в дальнейшем будем вычитывать сообщение и проверять его целостность и правильность
type TestEventStore struct {
	mu     sync.RWMutex // для контроля доступа горутин к переменным
	events []domain.UserEvent
}

// вызывается консьюмером
func (s *TestEventStore) Add(event domain.UserEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

// этот метод нужен, что бы используя его, в тестах находить ивент по определённому email
func (s *TestEventStore) FindByEmail(email string) (*domain.UserEvent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, e := range s.events {
		if e.Payload.Email == email {
			return &e, true
		}
	}
	return nil, false
}

// !! Эта глобальная переменная нужна для того, что бы при сборке консьюмера мы могли передавать метод Add
/* передаётся аргументом в NewConsumer

consumer := kafka.NewConsumer(testBrokers, testTopic, testGroupID, logger, func(event domain.UserEvent) error {
		testEventStore.Add(event)
		return nil
	},
	)
*/
var TestEventStoreGlobal = TestEventStore{}

// Сама по себе вся kafka работает асинхронно, а нам нужно уметь получать сообщения от kafka, для етого мы должны сделать бесконечный цикл чтения сообщений, с ожиданием до момента прихода сообщения, иначе программа может попытаться достать сообщение в момент, пока оно ещё не успело прийти (Мы букально говорим, я не буду пытаться извлекать сообщение сразу, я подожду пока оно придёт)
// ожидатель события с ограничением по времени
func waitForKafkaEvent(t *testing.T, targetEmail string) *domain.UserEvent {

	timeout := time.After(5 * time.Second) // максимальное время ожидания прихода сообщения от kafka

	ticker := time.NewTicker(100 * time.Millisecond) // создаём тикер, который будет стараться прочитать сообщение каждую секунду, если его не добавлять, программа может пытаться читать сообщение тысячи раз, это излишне нагрузит процессор

	for {
		select { // селект останавливает выполнение приостанавливает программу, на момент, пока ждёт какой канал первым сработает

		case <-timeout:
			t.Fatalf("Таймаут: сообщение для email %s так и не пришло в Kafka", targetEmail)
			return nil

		case <-ticker.C:
			if event, found := TestEventStoreGlobal.FindByEmail(targetEmail); found {
				return event
			}

		}
	} // ! если мы не нашли никакого сообщения, то select пойдёт на следующий круг и будет вновь ждать либо timeout остановки, либо каждые 100 миллисекунд. Всё повториться в случае не обнаружени сообщения

}

// Такой же метод как и waitForKafkaEvent, тольок этот может возвращать большое количество сообщений. Это нужно для теста о проверки порядка сообщений
func waitForKafkaEventBatch(t *testing.T, targetID int64, expectedCount int) []domain.UserEvent {
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	matchedEvents := []domain.UserEvent{} // перемнная куда будут помещаться все подходящие

	for {
		select {
		case <-timeout:
			t.Fatalf("Таймаут: не удалось получить %d ивентов с id пользователей %d удалось получить только %d пользователей в kafka", expectedCount, targetID, len(matchedEvents))
			return nil

		case <-ticker.C:

			matchedEvents = []domain.UserEvent{} // обнуляем слайс matchedEvents на каждой итерации, что бы не дублировать события

			TestEventStoreGlobal.mu.RLock()
			for _, e := range TestEventStoreGlobal.events { // за счёт глобальной переменной смотрим(перебираем циклом) вообще все созданные ивенты в ивент сторе
				if e.Payload.ID == targetID { // проверяем какие из всех ивентов подходят по id именно созданного в тесте пользователя, не ивента (targetID - это id пользователя, мы просто передаём в метод CreateUser.ID например)
					matchedEvents = append(matchedEvents, e)
				}
			}
			TestEventStoreGlobal.mu.RUnlock()

			if len(matchedEvents) >= expectedCount {
				return matchedEvents
			}
		}
	}
}

// Поскольку тесты запускаются параллельно (t.Parallel()), все они будут писать события в один топик test-topic-user-events. Чтобы тесты не мешали друг другу, нам нужно "складывать" все прочитанные консюмером события в потокобезопасное хранилище, где каждый тест сможет найти своё событие по уникальному Email или ID.

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
	consumer := kafka.NewConsumer(testBrokers, testTopic, testGroupID, logger, func(event domain.UserEvent) error {
		TestEventStoreGlobal.Add(event)
		return nil
	},
	)

	consumerCtx, canclConsumer := context.WithCancel(context.Background())

	go consumer.StartKafkaConsumer(consumerCtx)

	env = &TestEnv{
		Repo:          repo,
		Cache:         cache,
		KafkaProducer: producer,
		KafkaConsumer: consumer,
		Svc:           service.NewUserService(repo, cache, producer, logger, 5*time.Second),
	}

	code := m.Run()

	canclConsumer()

	if err := producer.Close(); err != nil {
		logger.Error("failed to close kafka producer", "err", err)
	}

	if err := consumer.Close(); err != nil {
		logger.Error("failed to close kafka consumer", "err", err)
	}

	_ = env.Cache.Close()
	_ = env.Repo.Close()

	os.Exit(code)
}
