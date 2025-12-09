package service

import (
	"context"

	"github.com/Derbik-Git/user-service/internal/domain"
)

type UserService interface {
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	CreateUser(ctx context.Context, email, name string) (*domain.User, error)
	UpdateUser(ctx context.Context, id int64, email, name string) (*domain.User, error)
	DeleteUser(ctx context.Context, id int64) error
}

type UserRepository interface {
	GetUserByID(ctx context.Context, id int64) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) (*domain.User, error)
	Delete(ctx context.Context, id int64) error
}

type userService struct {
	repo UserRepository
}

func NewUserService(repo UserRepository) UserService {
	return &userService{repo: repo}
}

func 