package server

import (
	"context"
	"log/slog"
	"os"
	"time"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
	"github.com/Derbik-Git/user-service/internal/sl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserService interface {
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	CreateUser(ctx context.Context, email, name string) (*domain.User, error)
	UpdateUser(ctx context.Context, u *domain.User) (*domain.User, error)
	DeleteUser(ctx context.Context, id int64) error
}

type Server struct {
	userv1.UnimplementedUserServiceServer
	UserService UserService
	logger      *slog.Logger
}

// нужно зарегстрировать grpc сервер, его нужно собрать в app.go
func RegisterServer(gRPC *grpc.Server, userService UserService, logger *slog.Logger) {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	userv1.RegisterUserServiceServer(gRPC, &Server{
		UserService: userService,
		logger:      logger,
	},
	)
}

func NewServer(userService UserService, logger *slog.Logger) *Server {

	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	return &Server{
		UserService: userService,
		logger:      logger,
	}
}

func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	const op = "app.Server.GetUser"

	if req == nil || req.GetId() <= 0 {
		s.logger.Warn("invalid request", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "id must be > 0")
	}

	usr, err := s.UserService.GetUser(ctx, req.GetId())
	if err != nil {
		s.logger.Warn("get user failed", slog.String("op", op), sl.Err(err))
		return nil, errorsx.ToGRPC(err)
	}

	return &userv1.GetUserResponse{
		User: &userv1.User{ // конвертируем структуру User в userv1.User(эта структура пришла из сервиса)
			Id:        usr.ID,
			Name:      usr.Name,
			Email:     usr.Email,
			CreatedAt: usr.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (s *Server) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	const op = "app.Server.CreateUser"

	if req == nil || req.GetEmail() == "" {
		s.logger.Warn("missing email", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	if req.GetName() == "" {
		s.logger.Warn("missing name", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	usr, err := s.UserService.CreateUser(ctx, req.GetEmail(), req.GetName())
	if err != nil {
		s.logger.Error("CreateUser failed", slog.String("op", op), sl.Err(err))
		return nil, errorsx.ToGRPC(err)
	}

	return &userv1.CreateUserResponse{
		User: &userv1.User{
			Id:        usr.ID,
			Email:     usr.Email,
			Name:      usr.Name,
			CreatedAt: usr.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (s *Server) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	const op = "app.Server.UpdateUser"

	// Проверка валидности запроса
	if req == nil || req.GetId() <= 0 {
		s.logger.Warn("invalid request: id must be > 0", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "id must be > 0")
	}
	if req.GetEmail() == "" && req.GetName() == "" {
		s.logger.Warn("invalid request: nothing to update", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "nothing to update")
	}

	// Формируем domain.User из данных запроса
	usrDomain := &domain.User{
		ID:    req.GetId(),
		Email: req.GetEmail(),
		Name:  req.GetName(),
	}

	// Вызываем сервис
	usr, err := s.UserService.UpdateUser(ctx, usrDomain)
	if err != nil {
		s.logger.Warn("UpdateUser failed", slog.String("op", op), sl.Err(err))
		return nil, errorsx.ToGRPC(err)
	}

	// Возвращаем ответ protobuf
	return &userv1.UpdateUserResponse{
		User: &userv1.User{
			Id:        usr.ID,
			Email:     usr.Email,
			Name:      usr.Name,
			CreatedAt: usr.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (s *Server) DeleteUser(ctx context.Context, req *userv1.DeleteUserRequest) (*userv1.DeleteUserResponse, error) {
	const op = "app.Server.DeleteUser"

	if req == nil || req.GetId() <= 0 {
		s.logger.Warn("invalid request", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "id must be > 0")
	}

	if err := s.UserService.DeleteUser(ctx, req.GetId()); err != nil {
		s.logger.Warn("DeleteUser failed", slog.String("op", op))
		return nil, errorsx.ToGRPC(err)
	}
	return &userv1.DeleteUserResponse{Success: true}, nil
}
