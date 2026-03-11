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
	client redis.Cmdable // теперь поддерживает и single, и cluster redis
	ttl    time.Duration
	logger *slog.Logger
}

// используется только в it тестах, для того что бы чистить редис после каждого теста
func (r *RedisCache) Client() redis.Cmdable { // благодаря этому методы мы возвращаем этот интерфейс redis.Cmdable, с помощью которого мы можем дёргать методы кеша, такие как GET, SET, DEL, TTL, FLUSHDB. Это redis client wrapper, который: использует connection pool, управляет reconnect
	return r.client
}

func NewRedisCache(addrs []string, ttl time.Duration, opts *redis.ClusterOptions, logger *slog.Logger) (*RedisCache, error) {
	const op = "cache.redis.NewRedisCache"

	if len(addrs) == 0 { // адреса, берутся из конфига
		logger.Error("no redis address provided", slog.String("op", op))
		return nil, errors.New("no redis address provided")
	}

	var client redis.Cmdable

	// если 1 адрес — используем обычный клиент
	if len(addrs) == 1 {
		client = redis.NewClient(&redis.Options{
			Addr: addrs[0],
		})
	} else {
		// если несколько адресов — кластер

		if opts == nil {
			// поверка дали ли мы кастомные опции, если нет, передаём структуру с опциями по умолчанию
			// (позже заполняем(в этом методе))
			opts = &redis.ClusterOptions{}
			// — это не “создание дефолтного кластера”. Это просто создание структуры опций
			// с нулевыми (дефолтными) полями библиотеки.
			// Эти поля библиотеки уже содержат разумные значения по умолчанию.
			// Мы потом обязательно заполним opts.Addrs.
		}

		opts.Addrs = addrs // помещаем адреса в структуру, тем самым говорим,
		// что мы хотим использовать эти адреса для подключения к редису

		client = redis.NewClusterClient(opts)
	}

	// !!!!!!!! Всё нужно прочитать потому что иначе программа просто бы завершилась
	/* Я ЗАКОМЕНТИРОВАЛ PING, Т.К. ЭТО МЕШАЕТ ЗАПУСКУ FALLBACK ТЕСТАМ REDIS, ПОТОМУ ЧТО ТАМ МЫ ДОЛЖНФ ПОПЫТАТЬСЯ ОТКРЫТЬ НЕ СУЩЕСТВУЮЩУЮ БД, ЧТО БЫ ТАК СКАЗАТЬ ВЫПОЛНИТЬ FALLBACK ТЕСТ БЕЗ REDIS, А ТАК У НАС ПРОГРАММА ПРОПИНГУЕТ СОЕДИНЕНИЕ И ВСЁ НА ЭТОМ ЗАКОНЧИТСЯ, ПОЭТОМУ ПИНГАВАТЬ МЫ БУДЕИ В MAIN.GO, А СОЕДИНЕНИЕ ПРОЙДЁТ НОРМАЛЬНО, ПРОСТО ПРИ ПЕРВОЙ ПОПЫТКЕ КЛАССИЧЕСКОГО ЗАПРОСА, В РЕДИС ПРОГРАММА ВЫДАСТ ОШИБКУ
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// в данном случае контекст создан для того, что бы остановить Ping
	// в случае долгого подключения, потому что если подключение идёт
	// дольше чем 5 секунд, значит что то не так и нам надо отключаться
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Ping - проверяет связь с редисом, команда отправляет в редис PING
		// он должен ответить PONG, если нет, значит что то помешало соединению
		logger.Error("redis PING failed", slog.String("op", op), slog.Any("err", err))
		return nil, err
	}
	*/

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

	cmd := c.client.Get(ctx, key) // в данном случе key - это ключь в самом redis, при вызове метода GetUser в тех же самых тестах передавая id сгенерированного пользователя, оно сначала за счёт userKey конвертируется в строку, потому что в redis ключи - это строки, потом попадает в метод Get (в качестве ключа для redis), который напрямую взаимодействует с redis-ом,
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

	var err error

	switch client := c.client.(type) {
	case *redis.Client:
		err = client.Close()
	case *redis.ClusterClient:
		err = client.Close()
	default:
		return nil
	}

	if err != nil {
		c.logger.Error("redis client close failed", slog.String("op", op), slog.Any("err", err))
		return err
	}

	return nil
}
