package app

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/Derbik-Git/user-service/internal/server"
	"google.golang.org/grpc"
)

type App struct {
	log        *slog.Logger
	gRPCServer *grpc.Server
	port       int
}

func NewApp(log *slog.Logger, userService server.UserService, port int) *App {
	gRPCServer := grpc.NewServer()

	server.RegisterServer(gRPCServer, userService, log)

	return &App{
		log:        log,
		gRPCServer: gRPCServer,
		port:       port,
	}
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

func (a *App) Run() error {
	const op = "app.Run"

	log := a.log.With(slog.String("op", op), slog.Int("port", a.port))

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", a.port))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	log.Info("grpc server is running", slog.String("addr", l.Addr().String()))

	if err := a.gRPCServer.Serve(l); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (a *App) Stop() {
	const op = "app.Stop"

	a.log.With(slog.String("op", op)).Info("grpc server is stopped", slog.Int("port", a.port))

	a.gRPCServer.GracefulStop()
}
