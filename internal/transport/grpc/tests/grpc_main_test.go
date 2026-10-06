package tests

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net" // для создания сетевого слушателя (TCP)
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/interceptors"
	"github.com/Derbik-Git/user-service/internal/migrate"
	"github.com/Derbik-Git/user-service/internal/repository/postgres"
	"github.com/Derbik-Git/user-service/internal/server"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const testJwtSecret = "test-secret-key-12345" // Секрет для тестов

var (
	client userv1.UserServiceClient // клиент для вызовов метода gRPC
	conn   *grpc.ClientConn         // соединение с сервером
)

// Helper: генерирует валидный JWT токен и упаковывает его в gRPC context
func authCtx(userID int64) context.Context {
	claims := jwt.MapClaims{
		"uid": float64(userID),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(testJwtSecret))

	md := metadata.Pairs("authorization", "Bearer "+tokenStr)
	return metadata.NewOutgoingContext(context.Background(), md)
}

func TestMain(m *testing.M) {
	fmt.Println(">>> DEBUG: TestMain ЗАПУСТИЛСЯ <<<")

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		// Подстраховка: дефолтный DSN для локального Docker
		dsn = "postgres://users_db:users_db@localhost:5433/users_db?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open db for migrations: %v", err)
	}

	migrationsPath := "../../../../migrations"

	if err := migrate.MigrateUp(db, migrationsPath); err != nil {
		log.Fatalf("failed to apply migrations in test: %v", err)
	}
	db.Close()

	postg, err := postgres.NewStorage(dsn)
	if err != nil {
		// Выводим РЕАЛЬНУЮ ошибку подключения %v
		log.Fatalf("failed to connect to postgres (dsn=%s): %v", dsn, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	kafkaBrokersStr := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokersStr == "" {
		kafkaBrokersStr = "127.0.0.1:9091,127.0.0.1:9092,127.0.0.1:9093"
	}
	testBrokers := strings.Split(kafkaBrokersStr, ",")

	redisAddrStr := os.Getenv("REDIS_ADDRS")
	if redisAddrStr == "" {
		redisAddrStr = "127.0.0.1:7001"
	}
	redisAddrs := strings.Split(redisAddrStr, ",")

	producer := kafka.NewProducer(testBrokers)
	cache, err := cache.NewRedisCache(redisAddrs, 3*time.Second, nil, logger)

	// сборка(инициализация) приложения:
	// _
	service := service.NewUserService(postg, cache, producer, logger, 5*time.Second) // создаётся экземпляр основной логики, с переданными зависимостями (хранилище, кэш, логгер и таймаут для кэша)

	newServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptors.AuthInterceptor(testJwtSecret)),
	)

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
