package cache

import (
	"context"
	"time"

	"github.com/Derbik-Git/user-service/internal/domain"
)

type Cache interface {
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	SetUser(ctx context.Context, u *domain.User, ttl time.Duration) error
	DeleteUser(ctx context.Context, id int64) error
}
