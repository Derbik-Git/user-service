package mocks

import (
	"context"
	"errors"
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
		return nil, errors.New("cache.GetUser was not expected to be called in this test") // допиши текст ошибок
	}

	return c.GetUserFunc(ctx, id)
}

func (c *CacheMock) SetUser(ctx context.Context, u *domain.User, ttl time.Duration) error {
	if c.SetUserFunc == nil {
		return errors.New("cahce.SetUser is not implemented for this test case")
	}

	return c.SetUserFunc(ctx, u, ttl)
}

func (c *CacheMock) DeleteUser(ctx context.Context, id int64) error {
	if c.DeleteUserFunc == nil {
		return errors.New("cahce.DeleteUser is not implementedfor this test case")
	}

	return c.DeleteUserFunc(ctx, id)
}
