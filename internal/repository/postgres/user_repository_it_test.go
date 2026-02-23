package postgres // нахождение интеграционного теста postgres, в том же пакете что и репозиторий - это нормальная практика, даже для другого вида интеграционных тестов

// t.Parralel() - значит что тест сможет выполнятся паралельно с другими пакетами, но не с самим собой

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/Derbik-Git/user-service/internal/repository/postgres/storage"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/require"
)

func cleanUsersTable(ctx context.Context, s *Storage) error {
	_, err := s.db.ExecContext(ctx, "TRUNCATE TABLE users RESTART IDENTITY CASCADE") // TRUNCATE — мгновенно очищает таблицу  RESTART IDENTITY — сбрасывает GENERATED AS IDENTITY  CASCADE — на будущее (если появятся FK)
	return err
}

func createRandomUser(ctx context.Context, s *Storage) (*domain.User, error) {
	return s.Create(ctx, gofakeit.Email(), gofakeit.Name())
}

func TestPostgres_UserRepository_Integration(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("POSTGRES_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := NewStorage(dsn)
	require.NoError(t, err) // если err != nil, то тест завершается с ошибкой
	defer store.Close()

	require.NoError(t, cleanUsersTable(ctx, store)) // Если результата вызова функции cleanUsersTable err != nil, то тест завершается с ошибкой

	t.Run("CreateAndGetUser", func(t *testing.T) {
		user, err := createRandomUser(ctx, store) // user - это domain.User, потому что createRandomUser возвращает *domain.User, а в createRandomUser мы возвращаем s.Create, который возвраащет domain.User, ну в Create ужепросто содаётся переменная с типом domain.User
		require.NoError(t, err)

		require.NotZero(t, user.ID)
		require.NotEmpty(t, user.Email)
		require.NotEmpty(t, user.Name)
		require.WithinDuration(t, time.Now(), user.CreatedAt, 2*time.Second) // сравниваем время создания из базы со временем сейчас, допустимая погрешность 2 секунды

		found, err := store.GetUserByID(ctx, user.ID)
		require.NoError(t, err)
		require.NotNil(t, found)

		// сравниваем что мы создали и что достали из базы
		require.Equal(t, user.ID, found.ID)
		require.Equal(t, user.Email, found.Email)
		require.Equal(t, user.Name, found.Name)
		require.True(t, user.CreatedAt.Equal(found.CreatedAt))
	})

	t.Run("CreateDublicateEmail", func(t *testing.T) {
		email := gofakeit.Email()

		_, err := store.Create(ctx, email, gofakeit.Name())
		require.NoError(t, err)

		_, err = store.Create(ctx, email, gofakeit.Name())
		require.NoError(t, err)
		require.ErrorIs(t, err, storage.ErrUserExists)
	})

	t.Run("GetUserNotFound", func(t *testing.T) {
		user, err := store.GetUserByID(ctx, 999999999)
		require.NoError(t, err)
		require.Nil(t, user) // ожидаем что пользователь не найден, и когда приходить return nil, nil, это успех для этого теста, это не ошибка!
	})

	t.Run("UpdateUser", func(t *testing.T) {
		user, err := createRandomUser(ctx, store)
		require.NoError(t, err)

		user.Email = gofakeit.Email()
		user.Name = gofakeit.Name()

		update, err := store.Update(ctx, user)
		require.NoError(t, err)

		require.Equal(t, user.ID, update.ID)
		require.Equal(t, user.Email, update.Email)
		require.Equal(t, user.Name, update.Name)
	})

	t.Run("UpdateDuplicateEmail", func(t *testing.T) {
		u1, err := createRandomUser(ctx, store)
		require.NoError(t, err)

		u2, err := createRandomUser(ctx, store)
		require.NoError(t, err)

		u2.Email = u1.Email

		_, err = store.Update(ctx, u2) // // мы тут будем менять второго пользователя, а не первого потому что id не меняется и мы уазываем что хотим поменять второго пользователя с данными от первого, потмоу что мы тут в коде их уже изменили
		require.Error(t, err)
		require.ErrorIs(t, err, storage.ErrUserExists) // в репозитории мы ошибку из бд маппим в ErrUserExists, поэтому тут что ошибка как раз таки ErrUserExists (!в репозитории! ошибка бд = ErrUserExists) туту проверяем что из репозитория пришла ErrUserExists
	})

	t.Run("DeleteUser", func(t *testing.T) {
		user, err := createRandomUser(ctx, store)
		require.NoError(t, err)

		require.NoError(t, store.Delete(ctx, user.ID))

		err = store.Delete(ctx, user.ID)
		require.ErrorIs(t, err, storage.ErrNotFound)
	})
}
