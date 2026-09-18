package clusterTest

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type ClusterEnv struct {
	Client *redis.ClusterClient // Позволяет обращатся к redis кластеру в ходе тестов
}

var env *ClusterEnv

func waitForRedisCluster(addr []string) (*redis.ClusterClient, error) {

	// Создаем клиент ОДИН раз. Внутри Нью-клиента остается защита от IPv6 [::1]
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: addr,
		NewClient: func(opt *redis.Options) *redis.Client {
			_, port, err := net.SplitHostPort(opt.Addr)
			if err == nil {
				// Подменяем ЛЮБОЙ адрес (localhost, ::1, 172.x.x.x) на 127.0.0.1, сохраняя порт
				opt.Addr = net.JoinHostPort("127.0.0.1", port)
			}
			return redis.NewClient(opt)
		},
	})

	var lastErr error
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := client.Ping(ctx).Err()
		cancel()

		if err == nil {
			return client, nil
		}

		lastErr = err
		time.Sleep(1 * time.Second)
	}

	// Если за 10 попыток не поднялся — закрываем клиент и отдаем РЕАЛЬНУЮ ошибку
	_ = client.Close()
	return nil, fmt.Errorf("redis cluster unavailable after 10 retries: %w", lastErr)
}

func TestMain(m *testing.M) {
	fmt.Println(">>> TEST MAIN ЗАПУСТИЛСЯ <<<")

	envAddr := os.Getenv("SERVICE_TESTREDIS_ADDR")

	addr := []string{}
	if envAddr != "" {
		addr = strings.Split(envAddr, ",")
	} else {
		addr = []string{
			"127.0.0.1:7001",
			"127.0.0.1:7002",
			"127.0.0.1:7003",
			"127.0.0.1:7004",
			"127.0.0.1:7005",
			"127.0.0.1:7006",
		}
	}

	client, err := waitForRedisCluster(addr)
	if err != nil {
		panic(err)
	}

	env = &ClusterEnv{
		Client: client,
	}
	// !
	code := m.Run() // запускает все тесты в пакете

	_ = env.Client.Close() // после завершения всех тестов закрывается соединение с Redis-cluster

	os.Exit(code) // программа завершается с тем же кодом, что и вернули тесты
}
