package postgres

import (
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/Derbik-Git/user-service/internal/migrate"
)

// Безумно важно понимать что для каждой базы данных свои миграции, если мы запустили тестовую базу, для неё открыли одни миграции и они создали одну таблицу, если открываем и передаём dsn для прод бд через GitHub Actions, то при запуске бд у нас создаётся уже другая бд(не тестовая), и для неё применяются уже те же, но новые(ещё одни) миграции и создаётся другая таблица нежели как для тестов через export dsn
func TestMain(m *testing.M) { // M - это пакет, используемый для запуска всех тестов в пакете автоматически
	// export POSTGRES_DSN=postgres://users_tests:users_tests@localhost:5433/users_tests?sslmode=disable | потом заустить тесты // Формат DSN: postgres://USER:PASSWORD@HOST:PORT/DB_NAME
	dsn := os.Getenv("POSTGRES_IT_TEST_DSN") // Получаем строку подключения из переменной POSTGRES_TEST_DSN (мы передали её командой export) В продакшене GitHub Actions будет выставлять переменную POSTGRES_IT_TEST_DSN
	if dsn == "" {
		log.Fatal("POSTGRES_IT_TEST_DSN is not set")
	}

	db, err := sql.Open("postgres", dsn) // Нужно исключительно для запуска миграций, такой же метод для запускам бд есть в репозитории в функции NewStorage
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	if err := db.Ping(); err != nil { // Проверяем подключение к бд(исключительно что бы потом применить для запуска миграций, это не бд в которой мы будем работать, мы просто проверяем запустилась ли вообще и всё ли нормально(ДЛЯ ЗАПУСКА МИГРАЦИЙ! см. метод ниже))
		log.Fatalf("ping db: %v", err)
	}

	if err := migrate.MigrateUp(db, "../../../migrations"); err != nil { // такой странный путь к миграциям, потому что что бы дойти от internal/repository/postgres дл папки migrations, нужно поднятся на три уровня(../../../migrations)
		log.Fatalf("migrate up: %v", err)
	}

	code := m.Run() // запускает все тесты в пакете и возвращает exit code, который говорит успешно прошли тесты или нет (0 = успешно, любой другой код = ошибки)

	_ = db.Close() // закрываем что бы соединение не осталось висеть, _ = используется что бы игнорировать возвращаемую ошибку т.к. в TestMain мы уже получили результат
	os.Exit(code)  // завершаем выполнение программы с кодом, который вернул m.Run()
}
