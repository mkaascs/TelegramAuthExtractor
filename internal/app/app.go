package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"os"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/app/server"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/delivery/rest"
	"telegram-auth-extractor/internal/infra/device"
	"telegram-auth-extractor/internal/infra/storage"
	"telegram-auth-extractor/internal/infra/telegram"
	"telegram-auth-extractor/internal/infra/tgnet"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
	"telegram-auth-extractor/internal/services/sessions"
	"telegram-auth-extractor/internal/services/workers"
)

type App struct {
	cfg config.Config
	log *slog.Logger

	server *server.Server
	worker *workers.Worker
	device *device.Device

	cancelWorker context.CancelFunc
}

func NewApp(cfg config.Config, log *slog.Logger) *App {
	repo := storage.NewMemoryRepository()

	dev := device.NewDevice(cfg.ADB)
	worker := workers.NewWorker(repo,
		dev,
		tgnet.NewParser(),
		telegram.NewVerifier(cfg.Telegram, log),
		log, cfg.Worker)

	svc := sessions.NewService(repo, worker)

	handlers := rest.NewHandler(svc)
	router := chi.NewRouter()
	apiv1.HandlerFromMuxWithBaseURL(handlers, router, "/api/v1")

	srv := server.NewServer(cfg, router)

	log.Info("application has been initialized")

	return &App{
		cfg:    cfg,
		log:    log,
		server: srv,
		worker: worker,
		device: dev,
	}
}

func (a *App) Run(ctx context.Context) error {
	if err := a.device.Prepare(ctx); err != nil {
		return fmt.Errorf("device prepare: %w", err)
	}

	workerCtx, cancel := context.WithCancel(context.Background())
	a.cancelWorker = cancel

	go a.worker.Run(workerCtx)
	a.log.Info("worker is running")

	a.log.Info("http server is running", slog.Int("port", int(a.cfg.Http.Port)))
	return a.server.Run()
}

func (a *App) MustRun(ctx context.Context) {
	if err := a.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.Error("failed to run http server", sloglib.Error(err))
		os.Exit(1)
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.log.Info("worker is stopping")
	if a.cancelWorker != nil {
		a.cancelWorker()
	}

	a.log.Info("http server is stopping")
	return a.server.Shutdown(ctx)
}
