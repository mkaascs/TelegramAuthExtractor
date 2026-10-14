package config

import (
	"errors"
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
	"log"
	"os"
	"time"
)

type EnvType string

const (
	EnvProd  EnvType = "prod"
	EnvDev   EnvType = "dev"
	EnvLocal EnvType = "local"
)

type Config struct {
	Env  EnvType `yaml:"env"`
	Http Http    `yaml:"http_server"`
}

type Http struct {
	Port         uint16        `yaml:"port" env:"HTTP_PORT" env-default:"8080"`
	WriteTimeout time.Duration `yaml:"write_timeout" env-default:"5s"`
	ReadTimeout  time.Duration `yaml:"read_timeout" env-default:"10s"`
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		log.Fatal(err)
	}

	return cfg
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		return nil, errors.New("CONFIG_PATH environment variable not set")
	}

	var cfg Config
	if err := cleanenv.ReadConfig(cfgPath, &cfg); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return &cfg, nil
}
