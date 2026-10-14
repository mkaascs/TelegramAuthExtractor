package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"telegram-auth-extractor/internal/app/server"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
)

type App struct {
	cfg    config.Config
	log    *slog.Logger
	server *server.Server
}

func NewApp(cfg config.Config, log *slog.Logger) *App {
	srv := server.NewServer(cfg, nil)
	log.Info("application has been initialized")

	return &App{cfg: cfg, log: log, server: srv}
}

func (a *App) Run() error {
	a.log.Info("http server is running", slog.Int("port", int(a.cfg.Http.Port)))
	return a.server.Run()
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.Error("failed to run http server", sloglib.Error(err))
		os.Exit(1)
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.log.Info("http server is stopping")
	return a.server.Shutdown(ctx)
}
