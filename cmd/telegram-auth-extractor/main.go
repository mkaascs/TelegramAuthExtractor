package main

import (
	"log/slog"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging"
)

func main() {
	cfg := config.MustLoad()
	log := logging.MustLoad(string(cfg.Env))

	log.Info("telegram-auth-extractor is starting", slog.String("env", string(cfg.Env)))

	log.Info("telegram-auth-extractor is stopped")
}
