package appassembling

import (
	"log/slog"

	"github.com/Derbik-Git/user-service/internal/app"
)

type App struct {
	GRPCSrv *app.App
}

func NewAppMain(log *slog.Logger, grpcPort int, storagePath string) *App
