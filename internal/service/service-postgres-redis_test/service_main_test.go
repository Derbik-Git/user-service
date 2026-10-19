package servPostgRedKafkaTest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"strconv"

	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/Derbik-Git/user-service/internal/migrate"
	"github.com/Derbik-Git/user-service/internal/repository/postgres"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	kafkaSegmentio "github.com/segmentio/kafka-go"
)

const (
	SERVICE_TEST_POSTGRES_DSN  = "SERVICE_TEST_POSTGRES_DSN"
	SERVICE_TEST_REDIS_ADDR    = "SERVICE_TEST_REDIS_ADDR"
	SERVICE_TEST_KAFKA_BROKERS = "SERVICE_TEST_KAFKA_BROKERS"
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
	mu                   sync.RWMutex // для контроля доступа горутин к переменным
	events               []domain.UserEvent
	seen                 map[string]bool // за счёт этой мапы будем проверять дубликаты kafka(работу бд на стороне другого микросервиса, типо мы проверяем идемпотентность отдельной таблицой, и возвращаем ошибку unique в случае повторной отправки одного и того же события, в ролит отдельной таблицы у нас выступает мапа, через неё будет проходить сверка подобных событий по ключу)
	EnforceDeduplication bool            // <--- Флаг: проверять ли дубликаты (нужен как переключатель, где использовать проверку на дубликат, а где нет)
}

// вызывается консьюмером
func (s *TestEventStore) Add(event domain.UserEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// На всякий случай защищаемся от nil-мапы
	if s.seen == nil {
		s.seen = make(map[string]bool)
	}

	key := fmt.Sprintf("%s-%s", event.Type, event.ID)

	// проверяет равен ли ключ true(то есть уже есть в мапе)
	// false - ключа нет, идём дальше, добавляем ключ, то есть что событие было обработано(эмитация таблицы идемпотентности на стороне следующего микросервиса)
	if s.EnforceDeduplication {
		if s.seen[key] {
			// если найден симулируем ошибку Unique от бд
			return fmt.Errorf("storage: duplicate event")
		}
	}
	s.seen[key] = true // если такого ключа небыло, то мы его добавляем в мапу, что бы следующий запрос наткнуля на блок if и возника ошибка уникальности типо от настоящей бд на стороне следующего микросервиса
	s.events = append(s.events, event)

	return nil
}

// Метод Reset() — это функция очистки и подготовки хранилища к новому тесту.
func (s *TestEventStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = []domain.UserEvent{}
	s.seen = make(map[string]bool)
	s.EnforceDeduplication = false
}

// этот метод нужен, что бы используя его, в тестах находить ивент по определённому email
func (s *TestEventStore) FindByEmail(email string) (*domain.UserEvent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := 0; i < len(s.events); i++ {
		if s.events[i].Payload.Email == email {
			return &s.events[i], true
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

	timeout := time.After(15 * time.Second) // максимальное время ожидания прихода сообщения от kafka

	ticker := time.NewTicker(100 * time.Millisecond) // создаём тикер, который будет стараться прочитать сообщение каждую секунду, если его не добавлять, программа может пытаться читать сообщение тысячи раз, это излишне нагрузит процессор
	defer ticker.Stop()

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
	timeout := time.After(15 * time.Second)
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
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			pingErr := repo.DB().PingContext(ctx)
			cancel()

			if pingErr == nil {
				return repo, nil
			}
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

// createKafkaTopic отвечает за программное создание топика в Kafka при старте тестов,
// чтобы исключить ошибку "Unknown Topic Or Partition".
func createKafkaTopic(brokerAddr, topicName string) {
	// 1. Устанавливаем TCP-соединение с произвольным брокером из списка для получения метаданных.
	conn, err := kafkaSegmentio.Dial("tcp", brokerAddr)
	if err != nil {
		log.Fatalf("Не удалось подключиться к Kafka для создания топика: %v", err)
	}
	defer conn.Close()

	// 2. Запрашиваем у брокера информацию о текущем контроллере кластера (главном узле).
	controller, err := conn.Controller()
	if err != nil {
		log.Fatalf("Не удалось получить контроллер Kafka: %v", err)
	}

	// 3. Подключаемся напрямую к контроллеру кластера, так как только он может создавать топики.
	controllerConn, err := kafkaSegmentio.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		log.Fatalf("Не удалось подключиться к контроллеру Kafka: %v", err)
	}
	defer controllerConn.Close()

	// 4. Описываем конфигурацию нового топика (имя, 1 партиция, 1 реплика для локальных тестов).
	topicConfigs := []kafkaSegmentio.TopicConfig{
		{
			Topic:             topicName,
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	}

	// 5. Выполняем запрос к контроллеру на создание топика(ов).
	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		// Если топик уже существует, это штатная ситуация, не валим тест
		log.Printf("Информация о создании топика (возможно, уже создан): %v", err)
	} else {
		log.Printf("Топик %s успешно создан программно!", topicName)
	}
}

func TestMain(m *testing.M) {

	if err := godotenv.Load("../../../.env"); err != nil {
		log.Println("Не удалось загрузить .env файл по пути ../../../.env, попробуем ../../.env")
	}

	dsn := os.Getenv(SERVICE_TEST_POSTGRES_DSN)
	if dsn == "" {
		log.Fatalf("%s not set", SERVICE_TEST_POSTGRES_DSN)
	}

	redisAddr := os.Getenv(SERVICE_TEST_REDIS_ADDR)
	if redisAddr == "" {
		log.Fatalf("%s not set", SERVICE_TEST_REDIS_ADDR)
	}
	redisAddrs := strings.Split(redisAddr, ",")

	testKafkaBrokers := os.Getenv(SERVICE_TEST_KAFKA_BROKERS)
	if testKafkaBrokers == "" {
		log.Fatalf("%s not set. You must provide a Kafka broker address for integration tests.", SERVICE_TEST_KAFKA_BROKERS)
	}
	testBrokers := strings.Split(testKafkaBrokers, ",")

	if err := waitForKafka(testBrokers); err != nil {
		log.Fatal(err)
	}

	time.Sleep(4 * time.Second)

	testTopic := domain.TopicUserEvents // "user-events"
	createKafkaTopic(testBrokers[0], testTopic)

	repo, err := waitForPostgres(dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := migrate.MigrateUp(repo.DB(), "../../../migrations"); err != nil { // переадём соединение с бд для запуска миграций, функция DB тупо возвращает уже созданное соединение с бд созданное методом waitForPostgres
		log.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cache, err := cache.NewRedisCache(redisAddrs, 5*time.Second, nil, logger)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := cache.Client().Ping(ctx).Err(); err != nil {
		logger.Warn("Redis недоступен, работаем без кеша", "err", err)
	}

	if err := cache.Client().Ping(ctx).Err(); err != nil {
		logger.Warn("Redis недоступен, работаем без кеша", "err", err)
	}

	// redis имеет историю, и когда он для одного теста волнился под определённым id, и ему позже для следующего теста прилетает тот же id, то он говорит, что сэтим id он работу уже выполнил, ждёт 5 секунд и падает
	testGroupID := "test-groupID-" + uuid.New().String()

	producer := kafka.NewProducer(testBrokers)
	consumer := kafka.NewConsumer(testBrokers, testTopic, testGroupID, logger, func(event domain.UserEvent) error {
		err := TestEventStoreGlobal.Add(event)
		if err != nil {
			// Если база/хранилище вернуло ошибку дубликата — сервис её проглатывает и возвращает nil
			return nil
		}
		return nil
	},
	)

	consumerCtx, canclConsumer := context.WithCancel(context.Background())

	go consumer.StartKafkaConsumer(consumerCtx)

	time.Sleep(2 * time.Second)

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
