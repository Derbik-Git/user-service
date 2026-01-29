package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Derbik-Git/user-service/internal/cache"
	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
	"github.com/Derbik-Git/user-service/internal/sl"
)

type UserRepository interface {
	GetUserByID(ctx context.Context, id int64) (*domain.User, error)
	Create(ctx context.Context, email, name string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) (*domain.User, error)
	Delete(ctx context.Context, id int64) error
}

// менять
type Service struct {
	repo  UserRepository
	cache cache.Cache
	log   *slog.Logger
	ttl   time.Duration
}

func NewUserService(repo UserRepository, cache cache.Cache, log *slog.Logger, ttl time.Duration) *Service {
	if log == nil { //используем этот блок повторно, не смотря на наличие его в хендлере, так как сервис может использоваться без хендлера, например в тестах
		log = slog.Default()
	}

	return &Service{
		repo:  repo,
		cache: cache,
		log:   log,
		ttl:   ttl,
	}
}

func (s *Service) CreateUser(ctx context.Context, email, name string) (*domain.User, error) {
	const op = "service.CreateUser"
	s.log.Info(op)

	if email == "" || name == "" {
		s.log.Error(op, sl.Err(errorsx.ErrInvalidInput))
		return nil, errorsx.ErrInvalidInput
	}

	u, err := s.repo.Create(ctx, email, name)
	if err != nil {
		s.log.Error(op, sl.Err(err))
		return nil, err
	}

	return u, nil
}

func (s *Service) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	const op = "service.GetUser"
	s.log.Info(op)

	if id <= 0 {
		s.log.Error(op, sl.Err(errorsx.ErrInvalidInput))
		return nil, errorsx.ErrInvalidInput
	}

	if s.cache != nil { //если подключение redis не = 0, работаем с кэшем
		u, err := s.cache.GetUser(ctx, id)
		if err != nil {
			s.log.Warn(op, sl.Err(err))
		} else if u != nil {
			return u, nil
		}
	}

	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		s.log.Error(op, sl.Err(err))
		return nil, err
	}

	if u == nil {
		return nil, nil
	}

	if s.cache != nil {
		if err := s.cache.SetUser(ctx, u, s.ttl); err != nil {
			s.log.Warn(op, sl.Err(err))
		}
	}

	return u, nil
}

func (s *Service) UpdateUser(ctx context.Context, u *domain.User) (*domain.User, error) {
	const op = "service.UpdateUser"
	s.log.Info(op)

	if u == nil || u.ID <= 0 || u.Email == "" || u.Name == "" {
		s.log.Error(op, sl.Err(errorsx.ErrInvalidInput))
		return nil, errorsx.ErrInvalidInput
	}

	updated, err := s.repo.Update(ctx, u)
	if err != nil {
		s.log.Error(op, sl.Err(err))
		return nil, err
	}

	if updated == nil {
		return nil, nil
	}

	if s.cache != nil {
		if err := s.cache.SetUser(ctx, updated, s.ttl); err != nil {
			s.log.Warn(op, sl.Err(err))
			return nil, err
		}
	}

	return updated, nil
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	const op = "service.DeleteUser"
	s.log.Info(op)

	if id <= 0 {
		s.log.Error(op, sl.Err(errorsx.ErrInvalidInput))
		return errorsx.ErrInvalidInput
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.log.Error(op, sl.Err(err))
		return err
	}

	if s.cache != nil {
		if err := s.cache.DeleteUser(ctx, id); err != nil {
			s.log.Warn(op, sl.Err(err))
			return err
		}
	}

	return nil
}
