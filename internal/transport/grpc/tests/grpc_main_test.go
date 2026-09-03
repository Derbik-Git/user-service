package tests

import (
	"fmt"
	"log"
	"log/slog"
	"net" // для создания сетевого слушателя (TCP)
	"os"
	"testing"
	"time"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/repository/postgres"
	"github.com/Derbik-Git/user-service/internal/server"
	"github.com/Derbik-Git/user-service/internal/service"
	"google.golang.org/grpc"
)

var (
	client userv1.UserServiceClient // клиент для вызовов метода gRPC
	conn   *grpc.ClientConn         // соединение с сервером
)

func TestMain(m *testing.M) {
	fmt.Println(">>> DEBUG: TestMain ЗАПУСТИЛСЯ <<<")

	dsn := os.Getenv("SERVICE_TEST_POSTGRES_DSN")
	postg, err := postgres.NewStorage(dsn)
	if err != nil {
		log.Fatal("SERVICE_TEST_POSTGRES_DSN not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cache, err := cache.NewRedisCache([]string{"localhost:6379"}, 3*time.Second, nil, logger)
	if err != nil {
		log.Fatal(err)
	}

	testKafkaBrokers := os.Getenv("SERVICE_TEST_KAFKA_BROKERS")
	testBrokers := []string{}
	if testKafkaBrokers != "" {
		testBrokers = []string{testKafkaBrokers}
	}

	producer := kafka.NewProducer(testBrokers)

	// сборка(инициализация) приложения:
	// _
	service := service.NewUserService(postg, cache, producer, logger, 5*time.Second) // создаётся экземпляр основной логики, с переданными зависимостями (хранилище, кэш, логгер и таймаут для кэша)

	newServer := grpc.NewServer()

	server.RegisterGRPCServer(newServer, service, logger) // логика регистрируется на этом сервере. Теперь сервер знает, как обрабатывать запросы к сервису пользователя.
	// _

	// # Для gRPC тестов:
	// $env:SERVICE_TEST_POSTGRES_DSN="postgres://users_db:users_db@localhost:5432/users_db?sslmode=disable"
	// создаём слушатель
	// слушатель — это объект, который «слушает» определённый сетевой адрес и порт, ожидая входящих соединений от клиентов. Как только появляется соединение, слушатель «принимает» его и возвращает объект соединения, через который можно обмениваться данными.
	// проще говоря, слушатель, открывает порт и ожидает запрососов, на таком то порту, а клиент на этом же порту будет отправлять запросы, и таким образом между ними происходит связь, слушатель кого то ожидает, клиент к нему приходим, потому что порт один и тот же указан в создании слушателя и в создании клиента, и устанавливается между ними свзяь, по которой клиент может передавать запросы
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	// Получаем адрес с занятьим динамическим портом (например, "127.0.0.1:58432")
	targetAddr := listener.Addr().String()

	// запускаем сервер, со слушателем, горутина нужна для непрерывной работы сервера
	go func() { // горутина обязательное условие, иначе после вызова этой функции, программа завершится
		if err := newServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// создаём клиентсвое соединение, по нему будет работать клиент, это как настройка для клиента
	// когда сервер уже запущен и благополучно работает, мы создаём к нему клитентское соединение, этот клиент будет использоваться в тестах для отправки запрососв на наш созданный выше, локальный сервер
	conn, err := grpc.Dial( // функция Dial устанавливает соединение с gRPC сервером по указанному адресу и с заданными опциями. Она возвращает объект ClientConn, который представляет собой клиентское соединение, через которое можно отправлять запросы к серверу.
		targetAddr,                      // указываем на каком порту будет отправлять запросы клиент
		grpc.WithInsecure(),             // — соединение без TLS (для тестов и локальной разработки).
		grpc.WithBlock(),                // — вызов блокируется, пока соединение не будет установлено или не истечёт таймаут.
		grpc.WithTimeout(3*time.Second), // — если сервер не отвечает за 3 секунды, попытка соединения завершится ошибкой.
	)
	if err != nil {
		log.Fatal(err)
	}

	client = userv1.NewUserServiceClient(conn) // создаём сам gRPC клиент, за счёт которого будем дёргать методы
	fmt.Printf(">>> DEBUG: client инициализирован? %v <<<\n", client != nil)

	code := m.Run()

	if err := producer.Close(); err != nil {
		log.Printf("failed to close producer: %v", err)
	}

	if err := postg.Close(); err != nil {
		log.Printf("failed to close postgres: %v", err)
	}

	if err := cache.Close(); err != nil {
		log.Printf("failed to close cache: %v", err)
	}

	conn.Close()
	newServer.Stop()

	os.Exit(code)
}
