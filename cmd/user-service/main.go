package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	appassembling "github.com/Derbik-Git/user-service/internal/app_main"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	// env:"GRPC_PORT" - искать значение в переменной окружения GRPC_PORT
	// env-default:"50051" - если переменной нет, использовать 50051 (как число!)
	GRPCPort int `env:"GRPC_PORT" env-default:"50051"`

	// env-required:"true" - если при запуске нет переменной POSTGRES_DSN, программа сразу упадет с ошибкой
	PostgresDSN string `env:"POSTGRES_DSN" env-required:"true"`

	// env:"REDIS_ADDRS" - cleanenv сам разобьет строку "host1,host2" на массив строк!
	RedisAddres []string `env:"REDIS_ADDRS"`

	KafkaBrokers []string `env:"KAFKA_BROKERS" env-default:"localhost:9092"`

	// env-default:"5m" - cleanenv сам распарсит строку "5m" в тип time.Duration (5 минут)
	CacheTTL time.Duration `env:"CACHE_TTL" env-default:"5m"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logger.Info("starting user-service")

	var cfg Config

	//мы передаём в метод структуру, он смотрит на её теги, "env:" и ищет такие же названия в переменных окружения, достаёт из этих переменных значения и кладёт в структуру в соответствии название тега - имя переменной окружения
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		// в случае отсутствия значений в обязательных переменных программа упадёт, что бы человек, запускающиё код, увидел какие переменный вообще должны быть, мы вызовем метод GetDescription, который принимает структуру и выводит, какие переменные окружения ожидает увидеть программа
		// метод пнринимает структуру и выводит поля, которые требует структура
		help, _ := cleanenv.GetDescription(&cfg, nil)
		logger.Error("failed to redi config", slog.String("err", err.Error()), slog.String("help", help))
		os.Exit(1)
	}

	logger.Info("condig load", slog.Int("port", cfg.GRPCPort))

	application, cleanup := appassembling.NewAppMain(logger, cfg.GRPCPort, cfg.PostgresDSN, cfg.RedisAddres, cfg.KafkaBrokers, cfg.CacheTTL, nil)
	defer func() {
		logger.Info("running cleanup tasks...")
		// Вызываем функцию cleanup(), которую нам вернул NewAppMain.
		// Она закроет Redis, Kafka и Postgres.
		if err := cleanup(); err != nil {
			logger.Error("cleanup failed", slog.String("err", err.Error()))
		} else {
			logger.Info("cleanup finished succefully")
		}
	}()

	// запускаем gRPC-сервис в отдельном потоке, если запустить без горутины, программа зависнет на этой строчке и никогда не перейдёт к коду ожидания остановки
	go func() {
		if err := application.GRPCSrv.Run(); err != nil {
			logger.Error("grpc server stopped with error", slog.String("err", err.Error()))
		}
	}()

	// Graceful shutdown

	// создаём канал, который принимает сигналы от опреационной системы, например
	stop := make(chan os.Signal, 1)

	// Дословно, если от операционной системы придёт сигнал типа SIGTERM или SIGINT, положи его в канал stop
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	// таким образом ожидаем, пока ОС даст сигнал о завершении, и как только в канале появляеться значение подходящего типа, оно записываеться в переменную signal, если ничего не приходит, ожидает, пока не придёт
	signal := <-stop
	logger.Info("received signal to stop", slog.String("signal", signal.String()))

	// Когда сигнал о завершении работы пришёл, программа продолжает своё выполнение и таким образом, сообщение пришло -> идём дальше -> а дальше у нас строка остановки сервиса
	application.GRPCSrv.Stop()

	logger.Info("graceful shutdown completed")
}
