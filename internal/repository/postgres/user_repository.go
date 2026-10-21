package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"

	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
)

type Storage struct {
	dbMaster *sql.DB // Для всех CRUD запросов
	dbSlave  *sql.DB // Реплика - только get запросы
}

func (s *Storage) DB() *sql.DB { // этот метод нужен исключительно для тестов, что бы мы возвращали соединение с db и могли его использовать для запуска миграций
	return s.dbMaster
}

func NewStorage(masterDSN, slaveDSN string) (*Storage, error) {
	const op = "storage.postgres.NewStorage"

	log.Println("Открытие соединения с мастером (PgBouncer)...")
	dbMaster, err := sql.Open("postgres", masterDSN)
	if err != nil {
		return nil, fmt.Errorf("%s: master open failed: %w", op, err)
	}

	ctx, cancelMaster := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelMaster()
	if err := dbMaster.PingContext(ctx); err != nil {
		_ = dbMaster.Close()
		return nil, fmt.Errorf("%s: master ping failed: %w", op, err)
	}
	log.Println("Мастер успешно ответил на пинг.")

	log.Println("Открытие соединения с репликой...")
	dbSlave, err := sql.Open("postgres", slaveDSN)
	if err != nil {
		_ = dbMaster.Close()
		return nil, fmt.Errorf("%s: slave open failed: %w", op, err)
	}

	ctx, cancelSlave := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelSlave()
	if err := dbSlave.PingContext(ctx); err != nil {
		_ = dbMaster.Close()
		_ = dbSlave.Close()
		return nil, fmt.Errorf("%s: slave ping failed: %w", op, err)
	}
	log.Println("Реплика успешно ответила на пинг.")

	return &Storage{
		dbMaster: dbMaster,
		dbSlave:  dbSlave,
	}, nil
}

func (s *Storage) Create(ctx context.Context, email, name string) (*domain.User, error) {
	const op = "storage.postgres.Create"

	query := `
	INSERT INTO users (email, name) VALUES ($1, $2) 
	RETURNING id, created_at 
	`
	// (Строка результата) "RETURNING" - возвращает значения, с которыми в будущем можно работать в коде (что мы и делаем в QueryRowContext)

	var u domain.User
	u.Email = email
	u.Name = name

	err := s.dbMaster.QueryRowContext(ctx, query, email, name).Scan(&u.ID, &u.CreatedAt) // при помощи помощи Scan достаём переменные из строки результата SQL запроса и записываем в указанные пееменные.
	if err != nil {
		//В INSERT / UPDATE мы проверяем PqError, потому что это ошибки бизнес-ограничений БД(например нарушение NOT NULL или нарушение уникальности). Обычно проверка типа: if errors.Is(err, sql.ErrNoRows) тут нету замысловатой логики в самом запросе и ошибка будет наипростейшая, пользователя просто нет, поэтому и такая простая обработка, нежели в сложных запросов, где могут произойти грубые ошибки, требующие более глубокой обработки как при INSERT / UPDATE
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // «Если ошибка, произошедшая при выполнении запроса, является ошибкой PostgreSQL и её SQLSTATE-код равен 23505 (нарушение уникальности) то обработай её специальным образом»
			return nil, fmt.Errorf("%s: %w", op, errorsx.ErrAlreadyExists) // Пользователь уже существует
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &u, nil
}

func (s *Storage) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	const op = "storage.postgres.GetUserByID"

	query := `
	SELECT id, email, name, created_at
	FROM users
	WHERE id = $1
	`
	//Тут по умолчанию возвращаются без RETURNING все значения, после команды SELECT

	var u domain.User

	err := s.dbSlave.QueryRowContext(ctx, query, id).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { // Это не PgError потому что бд не считает это ошибкой (не ошибка PostgreSQL)
			return nil, fmt.Errorf("%s: %w", op, errorsx.ErrNotFound)
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &u, nil
}

func (s *Storage) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	const op = "storage.postgres.Update"

	query := `
	UPDATE users 
	SET email = $1, name = $2
	WHERE id = $3
	RETURNING id, email, name, created_at
	`

	var update domain.User // Входные данные ≠ результат операции + Без указателя потому что нужна пустая струтура для записи результата SQL запроса

	err := s.dbMaster.QueryRowContext(ctx, query, user.Email, user.Name, user.ID).Scan(&update.ID, &update.Email, &update.Name, &update.CreatedAt) // Входные данные ≠ результат операции (ЭТО ВАЖНО, это я говорю к тому что если мы начали бы передавать в Scan входящие значения функции как в прошлых методах репозитория, Postgres начал бы добавлять результат sql запроса в не пустые поля структуры, а с какими то значениями, так как для метода Update передавалась заполненнная структура, а для корректного заполнения нам нужна пустая структура, что бы структура не заполнилась некорректными данными входящие параметры для запуска SQL запроса + его результат, это не корректно!!!! И выведет не тот результат SQL запроса, которйм мы ожидали получить, а будут некорректные данные и путаница!!!! Поэтому нужно создавать пустую структуру для записис SQL результата)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, fmt.Errorf("%s: %w", op, errorsx.ErrAlreadyExists) // Пользователь уже существует
		}

		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, errorsx.ErrNotFound) // Пользователь не найден
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &update, nil
}

func (s *Storage) Delete(ctx context.Context, id int64) error {
	const op = "storage.postgres.Delete"

	query := `
	DELETE FROM users
	WHERE id = $1
	`

	res, err := s.dbMaster.ExecContext(ctx, query, id) // Почему Exec, а не QueryRow/Scan: DELETE обычно не возвращает строки | нам не нужно читать данные пользователя | нам важно узнать: удалилось или нет
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := res.RowsAffected() // Показывает количество строк, которые были затронуты SQL запросом
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if affected == 0 { // если затронуто 0 строк, это значит что удалено 0 строк, это обозначает что пользователь не найден
		return fmt.Errorf("%s: %w", op, errorsx.ErrNotFound) // Пользователь не найден
	}

	return nil
}

// Метод Close нужен для того, что бы когда приложение закрывается по greceful shutdown, то нужно закрыть соединение, освободить рксурсы, не оставлять висящие коннекты, иначе на сервере могут копиться открытые соединения и Postgres может уперется в лимит открытых соединений max_connections
func (s *Storage) Close() error {
	var masterErr, slaveErr error

	if s.dbMaster == nil { // Что бы не ловить панику если репозиторий создался без подключения к бд
		masterErr = s.dbMaster.Close()
	}
	if s.dbSlave == nil {
		slaveErr = s.dbSlave.Close()
	}

	return errors.Join(masterErr, slaveErr)
}
