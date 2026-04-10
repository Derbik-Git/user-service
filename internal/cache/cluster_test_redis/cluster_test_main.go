package clusterTest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type ClusterEnv struct {
	Client *redis.ClusterClient // Позволяет обращатся к redis кластеру в ходе тестов
}

var env *ClusterEnv

func waitForRedisCluster(addr []string) (*redis.ClusterClient, error) {

	for i := 0; i <= 10; i++ {

		client := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: addr,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		err := client.Ping(ctx).Err()

		cancel() // ! не смотря на то что контекст сам завершится по истечению 3 секунд, cancel() необходимо делать что бы не оставалось лишних процессов | Так зачем вызывать  отдельно? Причина проста: это хорошая практика для предотвращения утечек памяти и освобождения ненужных ресурсов. Даже если таймаут сработал бы самостоятельно, использование cancel() гарантирует чистое завершение процесса и уменьшает нагрузку на систему.

		if err == nil {
			return client, nil
		}

		// не имеет смысла мгновенно повторять проверку, если предыдущая попытка закончилась неудачей. Пауза позволяет дать немного времени службе Redis окончательно запуститься и подготовиться к обработке запросов.
		time.Sleep(time.Second) // ожидается небольшая пауза (чтобы снизить нагрузку на сеть)
	}

	return nil, redis.ErrClosed
}

func TestMain(m *testing.M) {

	addr := []string{
		"localhost:7001",
		"localhost:7002",
		"localhost:7003",
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
