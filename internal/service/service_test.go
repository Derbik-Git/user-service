package service // unit тесты

import (
	"context"
	"errors"
	"testing"
	"time"

	"log/slog"

	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
	"github.com/Derbik-Git/user-service/internal/service"
	"github.com/Derbik-Git/user-service/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// В этом, саом первом тесте даны грамотные объяснения как работает тестирование
func TestService_CreateUser(t *testing.T) {
	t.Parallel() // объявляем что тесты могут выполнятся параллельно

	ctx := context.Background() // создаём контекст, просто потому что этого требует логика сервиса

	tests := []struct {
		nameTest      string
		emailArgument string
		nameArgument  string
		repo          *mocks.UserRepositoryMock
		cache         *mocks.CacheMock
		wantErr       error // ожидание что вернёт тест, тут идёт сверка, то ли врнул тест или нет(аргумент, который мы ожидаем)
	}{
		{
			nameTest:      "success",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			repo: &mocks.UserRepositoryMock{
				CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) { // говорим что от этого метода ожидаем вот такой вот return(строчка ниже)
					return &domain.User{ID: 1, Email: email, Name: name}, nil // мы лишь описываем что должно вернутся в случае если сервис вызовет CreateFunc, то есть если при запуске теста именно сервис вызовет CreateFunc в моке, так как настоящий репозиторий ничего не может вернуть, так как это Unit тесты и у нас стоит мок, мы говорим что при вызове такой то функции из мока &mocks.UserRepositoryMock{CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error), хотим увидеть вот такой результат return &domain.User{ID: 1, Email: email, Name: name}, nil
				},
			},
			cache:   nil,
			wantErr: nil,
		},
		{
			nameTest:      "invalid input",
			emailArgument: "",
			nameArgument:  "",
			repo:          &mocks.UserRepositoryMock{}, // тут программа даже не доходит типо до репозитория, она падает уже в сервисе, потому что ввод не верный, эта логика указана в сервисе, тио если email == "" || name == "", то возвращаем и errorsx.ErrInvalidArgument, поэтому и тут всё так, ожидаем именно эту ошибку и не вызываем как бы вот такой метод мока CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) как в сосседних тестах, потому что программа даже до него не доходит, а падает на проверке if == "" уже в сервисе не доходя до репозитория
			wantErr:       errorsx.ErrInvalidInput,
		},
		{
			nameTest:      "repo error",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			repo: &mocks.UserRepositoryMock{
				CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
					return nil, errors.New("db error") // а тут мы ожидаем ошибку именно от репозитория, поэтому и вызываем мок метод репозитория(ну то есть говорим что хотим от него получить)
				},
			},
			// cache: nil,
			wantErr: errors.New("db error"),
		},
	}

	for _, tt := range tests {
		tt := tt                              // дословно мы туту говорим Скопируй содержимое листа tt и положи его на новый отдельный лист, который будет жить только в этой итерации, это нужно для того что бы у каждой горутины/теста был свой собственный лист, как я понял, в кратце мы создаём дял каждого теста свою копию, что бы тесты не трогали один и тот же лист, без этого все тетсы читали бы последний тест кейс, а с этой конструкцией, каждй тест читает свой тест кейс
		t.Run(tt.nameTest, func(*testing.T) { // говорим чтот запускаем тесты с таким именем tt.nameTest
			t.Parallel()                                                          // объявляем что тесты могут выполнятся параллельно
			svc := NewUserService(tt.repo, tt.cache, slog.Default(), time.Minute) // даём сервису данные в конструктор для вызова/теста определённого метода
			u, err := svc.CreateUser(ctx, tt.emailArgument, tt.nameArgument)      // вызываем сам метод

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}
		})
	}
}

func TestService_GetUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		nameTest string
		id       int64
		repo     *mocks.UserRepositoryMock
		cache    *mocks.CacheMock
		wantErr  error
		wantNil  bool // нужно для того что бы, отличать случаи, когда мы ожидаем пользователь не найден, то есть когда мы ожидаем return nil, nil, wantNil = true, так же он всегда будет true, в любых тестах, где мы ожидаем ошибку, где мы ожидаем результат, то есть пользователь есть, wantNil всегда будет = false
	}{
		{
			nameTest: "from cache",
			id:       1,
			repo:     &mocks.UserRepositoryMock{},
			cache: &mocks.CacheMock{
				GetUserFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return &domain.User{ID: id, Email: "1@email.com", Name: "Test"}, nil
				},
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			nameTest: "repo return error",
			id:       2,
			repo: &mocks.UserRepositoryMock{
				GetUserByIDFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, errors.New("db error")
				},
			},
			cache:   &mocks.CacheMock{},
			wantErr: errors.New("db error"),
			wantNil: true,
		},
		{
			nameTest: "repo returns nil", // отсутствие пользователя по указанному id
			id:       3,
			repo: &mocks.UserRepositoryMock{
				GetUserByIDFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, nil
				},
			},
			cache:   &mocks.CacheMock{},
			wantErr: nil,
			wantNil: true,
		},
		{
			nameTest: "invalid id",
			id:       0,
			repo:     &mocks.UserRepositoryMock{},
			cache:    &mocks.CacheMock{},

			wantErr: errorsx.ErrInvalidInput,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(*testing.T) {
			t.Parallel()
			svc := NewUserService(tt.repo, tt.cache, slog.Default(), time.Minute)
			u, err := svc.GetUser(ctx, tt.id)

			if tt.wantErr == nil { //если в этом тесте мы не ждём ошибку
				require.NoError(t, err) // если ошибка есть, тест сразу падает

				if tt.wantNil { // ожидаем что результат будет nil
					assert.Nil(t, u) // проверяем что пользователь не существует
				} else {
					assert.NotNil(t, u) // иначе проверяем что пользователь существует
				}
			} else {
				require.Error(t, err)                            // если ожидаем ошибку, проверяем что она есть
				assert.Equal(t, tt.wantErr.Error(), err.Error()) // проверяем что ошибка совпадает с ожидаемой
			}
		})

	}
}

func TestService_UpdateUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name    string
		user    *domain.User
		repo    *mocks.MockUserRepository
		cache   *mocks.MockCache
		wantErr error
	}{
		{
			name: "success",
			user: &domain.User{ID: 1, Email: "a@b.com", Name: "John"},
			repo: &mocks.MockUserRepository{
				UpdateFn: func(ctx context.Context, u *domain.User) (*domain.User, error) {
					return u, nil
				},
			},
			cache:   nil,
			wantErr: nil,
		},
		{
			name:    "invalid input",
			user:    nil,
			repo:    &mocks.MockUserRepository{},
			cache:   nil,
			wantErr: errorsx.ErrInvalidInput,
		},
		{
			name: "repo error",
			user: &domain.User{ID: 1, Email: "a@b.com", Name: "John"},
			repo: &mocks.MockUserRepository{
				UpdateFn: func(ctx context.Context, u *domain.User) (*domain.User, error) {
					return nil, errors.New("update failed")
				},
			},
			cache:   nil,
			wantErr: errors.New("update failed"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := service.NewUserService(tt.repo, tt.cache, slog.Default(), time.Minute)
			u, err := svc.UpdateUser(ctx, tt.user)

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}
		})
	}
}

// ==========================
// DELETE USER
// ==========================
func TestService_DeleteUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		repo    *mocks.MockUserRepository
		cache   *mocks.MockCache
		wantErr error
	}{
		{
			name: "success",
			id:   1,
			repo: &mocks.MockUserRepository{
				DeleteFn: func(ctx context.Context, id int64) error { return nil },
			},
			cache:   nil,
			wantErr: nil,
		},
		{
			name:    "invalid id",
			id:      0,
			repo:    &mocks.MockUserRepository{},
			cache:   nil,
			wantErr: errorsx.ErrInvalidInput,
		},
		{
			name: "repo error",
			id:   1,
			repo: &mocks.MockUserRepository{
				DeleteFn: func(ctx context.Context, id int64) error { return errors.New("delete failed") },
			},
			cache:   nil,
			wantErr: errors.New("delete failed"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := service.NewUserService(tt.repo, tt.cache, slog.Default(), time.Minute)
			err := svc.DeleteUser(ctx, tt.id)

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}
		})
	}
}
