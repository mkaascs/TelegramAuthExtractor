package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"telegram-auth-extractor/internal/app/postgres"
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

	server     *server.Server
	worker     *workers.Worker
	dispatcher *workers.Dispatcher
	device     *device.Device
	db         *postgres.DB

	cancelWorkers context.CancelFunc
}

func NewApp(cfg config.Config, log *slog.Logger) *App {
	pg, err := postgres.NewDB(cfg.Postgres, log)
	if err != nil {
		os.Exit(1)
	}

	pg.MustConnect()

	sessionRepo := storage.NewSession(pg.Pool, log)
	resultRepo := storage.NewResult(pg.Pool, log)

	dev := device.NewDevice(cfg.ADB, log)
	worker := workers.NewWorker(sessionRepo,
		dev,
		tgnet.NewParser(),
		telegram.NewVerifier(cfg.Telegram, log),
		log, cfg.Service.Worker)

	dispatcher := workers.NewDispatcher(sessionRepo, worker, log, cfg.Service.Dispatcher)

	svc := sessions.NewService(sessionRepo, resultRepo, worker, cfg.Service)
	srv := server.NewServer(rest.NewHandler(svc), cfg, log)

	log.Info("application has been initialized")

	return &App{
		cfg:        cfg,
		log:        log,
		server:     srv,
		worker:     worker,
		dispatcher: dispatcher,
		device:     dev,
		db:         pg,
	}
}

func (a *App) Run(ctx context.Context) error {
	if err := a.device.Prepare(ctx); err != nil {
		return fmt.Errorf("device prepare: %w", err)
	}

	workerCtx, cancel := context.WithCancel(context.Background())
	a.cancelWorkers = cancel

	if err := a.dispatcher.Recover(workerCtx); err != nil {
		cancel()
		return fmt.Errorf("recover sessions: %w", err)
	}

	go a.worker.Run(workerCtx)
	go a.dispatcher.Run(workerCtx)

	a.log.Info("worker and dispatcher are running",
		slog.Int("queue_size", a.cfg.Service.Worker.QueueSize),
		slog.Int("batch_size", a.cfg.Service.Dispatcher.BatchSize))

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
	a.log.Info("worker and dispatcher are stopping")
	if a.cancelWorkers != nil {
		a.cancelWorkers()
	}

	a.log.Info("http server is stopping")
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}

	if a.db != nil {
		a.log.Info("database pool is closing")
		a.db.Close()
	}

	return nil
}
