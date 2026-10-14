package logging

import (
	"errors"
	"log"
	"log/slog"
	"os"
)

type EnvType string

const (
	EnvProd  EnvType = "prod"
	EnvDev   EnvType = "dev"
	EnvLocal EnvType = "local"
)

func MustLoad(env string) *slog.Logger {
	logger, err := Load(env)
	if err != nil {
		log.Fatal(err)
	}

	return logger
}

func Load(env string) (*slog.Logger, error) {
	var logger *slog.Logger

	switch EnvType(env) {
	case EnvLocal:
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case EnvDev:
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	case EnvProd:
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	default:
		return nil, errors.New("unknown environment type: " + env)
	}

	return logger, nil
}
