package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"telegram-auth-extractor/internal/app"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
	"time"
)

func main() {
	cfg := config.MustLoad()
	log := logging.MustLoad(string(cfg.Env))

	log.Info("telegram-auth-extractor is starting", slog.String("env", string(cfg.Env)))

	application := app.NewApp(*cfg, log)
	go application.MustRun(context.Background())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := application.Shutdown(ctx); err != nil {
		log.Error("failed to shutdown gracefully", sloglib.Error(err))
	}

	log.Info("telegram-auth-extractor is stopped")
}
