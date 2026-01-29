package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/Derbik-Git/user-service/internal/sl"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.ClusterClient
	ttl    time.Duration
	logger *slog.Logger
}

func NewRedisCache(addrs []string, ttl time.Duration, opts *redis.ClusterOptions, logger *slog.Logger) (*RedisCache, error) {
	const op = "cache.redis.NewRedisCache"

	if len(addrs) == 0 { // адреса, берутся из конфига
		logger.Error("no redis address provided", slog.String("op:", op))
		return nil, errors.New("no redis addres provided")
	}

	if opts == nil { //поверка дали ли мы кастомные опции, если нет, передаём структуру с опциями поумолчанию(позже заполняем(в этом методе))
		opts = &redis.ClusterOptions{} //— это не “создание дефолтного кластера”. Это просто создание структуры опций с нулевыми (дефолтными) полями библиотеки. Эти поля библиотеки уже содержат разумные значения по умолчанию (если библиотека их использует). Мы потом обязательно заполним opts.Addrs.
	}

	opts.Addrs = addrs //помещаем адреса в структуру, тем самым говорим, что мы хотим использовать эти адреса для подключения к редису

	client := redis.NewClusterClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //в данном случае контекст создан для того, что бы отсановить Ping в случае долгого подключения, потому что если подключение идёт дольше чем 5 секунд, значит чтото нетак и нам надо отключатся
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil { //Ping - проверяет связь с редисом, команда отправляет в редис PING он должен ответить PONG, если нет, значит что то помешало соединению, например кластер не отвечает | .Err() - используется что бы вернуть ошибку
		logger.Error("redis PING failed", slog.String("op:", op), sl.Err(err))
		return nil, err
	}

	return &RedisCache{
		client: client,
		ttl:    ttl,
		logger: logger,
	}, nil
}

func userKey(id int64) string { //в редисе ключи - это строки(эта функция переводит инт в строку для редиса + "user:" - префикс для сортировки ключей)
	return "user:" + strconv.FormatInt(id, 10)
}

func (c *RedisCache) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	const op = "cache.redis.GetUser"

	key := userKey(id)

	cmd := c.client.Get(ctx, key)
	b, err := cmd.Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) { //redis.Nil - ключ существует, но значеник пустое
			return nil, nil //дословно, передаём что всё ок, но значение по ключу пустое
		}
		c.logger.Error("redis GET failed", slog.String("op:", op), slog.String("key:", key), sl.Err(err))
		return nil, err
	}

	var u domain.User
	if err := json.Unmarshal(b, &u); err != nil {
		c.logger.Error("umarshal failed", slog.String("op:", op), slog.String("key:", key), sl.Err(err))
		return nil, err
	}
	return &u, nil
}

func (c *RedisCache) SetUser(ctx context.Context, u *domain.User, ttl time.Duration) error {
	const op = "cache.redis.SetUser"

	if u == nil {
		return nil
	}

	b, err := json.Marshal(u)
	if err != nil {
		c.logger.Error("marshal failed", slog.String("op:", op), slog.Int64("user_ID:", u.ID), sl.Err(err))
		return err
	}

	if ttl <= 0 {
		ttl = c.ttl //таким образом ttl берётся из конфига
	}

	if err := c.client.Set(ctx, userKey(u.ID), b, ttl).Err(); err != nil {
		c.logger.Error("redis SET failed", slog.String("op:", op), slog.Int64("user_ID:", u.ID), sl.Err(err))
		return err
	}
	return nil
}

func (c *RedisCache) DeleteUser(ctx context.Context, id int64) error {
	const op = "cache.redis.DeleteUser"

	if err := c.client.Del(ctx, userKey(id)).Err(); err != nil {
		c.logger.Error("redis DEL failed", slog.String("op", op), sl.Err(err))
		return err
	}
	return nil
}

func (c *RedisCache) Close() error {
	const op = "cache.redis.Close"

	if err := c.client.Close(); err != nil {
		c.logger.Error("redis client close failed", slog.String("op:", op), sl.Err(err))
		return err
	}
	return nil
}
