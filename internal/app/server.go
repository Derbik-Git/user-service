package app

import (
	"context"
	"log/slog"
	"os"

	userv1 "github.com/Derbik-Git/protos-tren-redis/user/v1"
	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
	"github.com/Derbik-Git/user-service/internal/sl"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/internal/status"
	//"github.com/Derbik-Git/protos-tren-redis/internal/errorsx"
	//"github.com/Derbik-Git/protos-tren-redis/internal/service"
)

type UserService interface {
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	CreateUser(ctx context.Context, email, name string) (*domain.User, error)
	UpdateUser(ctx context.Context, id int64, email, name string) (*domain.User, error)
	DeleteUser(ctx context.Context, id int64) error
}

type Server struct {
	userv1.UnimplementedUserServiceServer
	userService UserService
	logger      *slog.Logger
}

func NewServer(userService UserService, logger *slog.Logger) *Server {

	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	return &Server{
		userService: userService,
		logger:      logger,
	}
}

func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	const op = "app.Server.GetUser"

	if req == nil || req.GetId() <= 0 {
		s.logger.Warn("invalid request", slog.String("op", op))
		return nil, status.Error(codes.InvalidArgument, "id must be > 0")
	}

	u, err := s.userService.GetUser(ctx, req.GetId())
	if err != nil {
		s.logger.Warn("get user failed", slog.String("op", op), sl.Err(err))
		return nil, errorsx.ToGRPC(err)

	}

}
