package migrate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func MigrateUp(db *sql.DB, migrationsPath string) error {
	const op = "migrate.MigrateUp"

	cfg := &postgres.Config{}

	driver, err := postgres.WithInstance(db, cfg) // драйер для соединения миграций с базой данных
	if err != nil {
		return fmt.Errorf("%s: create drive failed: %w", op, err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver) //Этим методом создаётся интерфейс для управления миграциями, указывается путь к миграциям, и драйвер, в котором соединение с бд
	if err != nil {
		return fmt.Errorf("%s: create migrate instance failed: %w", op, err)
	}

	if err := m.Up(); err != nil { // запускаем миграции через интерфейс m для управления миграциями
		if errors.Is(err, migrate.ErrNoChange) { // migrate.ErrNoChange - это означает если никаких новых миграций нет, и схема базы данных уже актуальна, тогда функция завершается нормально
			return nil
		}

		return fmt.Errorf("%s: migrate up failed: %w", op, err)
	}

	return nil
}
