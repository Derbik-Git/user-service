package mocks

import (
	"context"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
)

type CacheMock struct {
	GetUserFunc    func(ctx context.Context, id int64) (*domain.User, error)
	SetUserFunc    func(ctx context.Context, u *domain.User, ttl time.Duration) error
	DeleteUserFunc func(ctx context.Context, id int64) error
}

func (c *CacheMock) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	if c.GetUserFunc == nil {
		return nil, nil
	}

	return c.GetUserFunc(ctx, id)
}

func (c *CacheMock) SetUser(ctx context.Context, u *domain.User, ttl time.Duration) error {
	if c.SetUserFunc == nil {
		return nil

	}

	return c.SetUserFunc(ctx, u, ttl)
}

func (c *CacheMock) DeleteUser(ctx context.Context, id int64) error {
	if c.DeleteUserFunc == nil {
		return nil
	}

	return c.DeleteUserFunc(ctx, id)
}

/*
Если вам нужно жестко контролировать вызовы, то в каждом тест-кейсе в service_unit_test.go вы обязаны передавать функции реализации:

func TestService_UpdateUser(t *testing.T) {
    cacheMock := &mocks.CacheMock{
        SetUserFunc: func(ctx context.Context, u *domain.User, ttl time.Duration) error {
            // Передаем nil, если хотим, чтобы запись в кэш прошла успешно
            return nil
        },
    }

    svc := service.NewUserService(repoMock, cacheMock, brokerMock, nil, time.Minute)
    // ... дальнейший вызов svc.UpdateUser(...)
}
*/
