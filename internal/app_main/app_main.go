package appassembling

import (
	"log/slog"
	"time"

	"github.com/Derbik-Git/user-service/internal/app"
	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/repository/postgres"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/redis/go-redis/v9"
)

type App struct {
	GRPCSrv *app.App
}

func NewAppMain(log *slog.Logger, grpcPort int, postgresDSN string, redisAddrs []string, cacheTTL time.Duration, opts *redis.ClusterOptions) (*App, func() error) {
	const op = "app_main.NewAppMain"

	if log == nil {
		log = slog.Default()
	}

	repo, err := postgres.NewStorage(postgresDSN)
	if err != nil {
		panic(err)
	}

	var (
		cacheInterface cache.Cache  // ВАЖНО ДЛЯ ПОНИМАНИЯ! любой объект, имеющий методы этого интерфейса, является кешем. ВАЖНО! Соотвтетственно, если эта переменная = nil, то кэша нет, программа продолжает работу без redis (по умолчанрию она nil)
		cacheClose     func() error // переменная, которая может хранить функцию для закрытия кеша
	)

	// тут логика пропуска или работы с кешем, то есть если успешно удалось создать кеш(структуру cache.RedisCache под капотом), то мы присваиваем переменной cacheInterface объект redisCache, тем самым интерфейс связывается со структурой и мы можем дергать через этот интерфейс кеш, если не удалось создать кеш, то мы присваиваем переменной cacheInterface значение nil и программа продолжает работу без redis(кеша)
	if len(redisAddrs) > 0 {
		redisCache, err := cache.NewRedisCache(redisAddrs, cacheTTL, opts, log)
		if err != nil {
			log.Warn("redis disabled, service wil run without cache", slog.String("op", op), slog.String("err", err.Error()))
		} else {
			cacheInterface = redisCache   // если всё хорошо, ссылаем cacheInterface на структуру redisCache, тем самым интерфейс связывается со структурой и мы таким образом даём доступ интерфейсу к структуре, что бы через интерфес можно было дёргать методы
			cacheClose = redisCache.Close // если редис есть, мы присваеваем этой переменной функцию, для закрытия кеша | redisCache.Close - это ссылка на функцию, а не её вызов
		}
	}

	userService := service.NewUserService(repo, cacheInterface, log, cacheTTL) // тут передаём кеш интерейс в сервис, где и будет логика работы с редисом, соответственно если интерфейс не узнал о структуре, реализующей эти методы(логика чуть выше), кеша не будут включены в работу

	grpcApp := app.NewApp(log, userService, grpcPort) // ВОЗВРАЩАЕТ струтктуру, которую app_main.go должен заполнить, и передавть в свою структуру App с параметром экземляра структуры из app.go, тем амым app.go передаёт струткуру, которую нужнозаполнить, что бы он работал, мы заполняем с данными из конфига в main.go, и передаём обратно в app.go через структуру в app_main.go

	application := &App{
		GRPCSrv: grpcApp,
	}

	// эта функци будет вызываться в main.go с defer (defer cleanup())
	cleanup := func() error {
		var err error

		if cacheClose != nil { // если cacheClose, не был создан, значит редиса просто нету и условие не выполняется
			if e := cacheClose(); e != nil { //если redis был, мы его сразу после этого аккуратно закрывем по завершении задачи редисом, во избежании утечки соединений/памяти
				err = e // если произошла ошибка закрытия redis, то сохраняем ее в err, что бы метод cleanup() мог её вернуть
			}
		}

		if e := repo.Close(); e != nil && err == nil { // по звершению работы репозитория, закрываем пул соединений Postgres, тем самым освоюождаются TCP-соединения
			err = e
		}

		return err
	}

	return application, cleanup

}

// использовать в main:
/*
app, cleanup := app.NewAppMain(...)

defer func() {
 if err := cleanup(); err != nil {
  log.Error("cleanup failed", slog.String("err", err.Error()))
 }
}()

app.MustRun()
*/
