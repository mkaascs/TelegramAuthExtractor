package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type EnvType string

const (
	EnvProd  EnvType = "prod"
	EnvDev   EnvType = "dev"
	EnvLocal EnvType = "local"
)

type Config struct {
	Env      EnvType  `yaml:"env"`
	Http     Http     `yaml:"http_server"`
	ADB      ADB      `yaml:"adb"`
	Service  Service  `yaml:"service"`
	Postgres Postgres `yaml:"postgres"`
	Telegram Telegram `yaml:"-"`
}

type Http struct {
	Port         uint16        `yaml:"port" env:"HTTP_PORT" env-default:"8080"`
	WriteTimeout time.Duration `yaml:"write_timeout" env-default:"5s"`
	ReadTimeout  time.Duration `yaml:"read_timeout" env-default:"10s"`
}

type Service struct {
	SessionTTL time.Duration `yaml:"session_ttl" env-default:"168h"`
	Worker     Worker        `yaml:"worker"`
	Dispatcher Dispatcher    `yaml:"dispatcher"`
}

type Worker struct {
	CodeWait       time.Duration `yaml:"code_wait" env-default:"1m"`
	CleanupTimeout time.Duration `yaml:"cleanup_timeout" env-default:"1m"`
	QueueSize      int           `yaml:"queue_size" env-default:"20"`
}

type Dispatcher struct {
	PollInterval time.Duration `yaml:"poll_interval" env-default:"2s"`
	BatchSize    int           `yaml:"batch_size" env-default:"10"`
}

type ADB struct {
	WaitTimeout       time.Duration `yaml:"wait_timeout" env-default:"30s"`
	PhoneCheckTimeout time.Duration `yaml:"phone_check_timeout" env-default:"30s"`
	WaitInterval      time.Duration `yaml:"wait_interval" env-default:"1s"`
	Package           string        `yaml:"package" env-required:"true"`
	ApkURL            string        `yaml:"apk_url" env-required:"true"`
	Binary            string        `yaml:"binary" env-default:"adb"`
	Serial            string        `yaml:"serial" env-required:"true"`
	NeedsConnect      bool          `yaml:"needs_connect"`
}

type Postgres struct {
	Addr              string        `yaml:"addr" env-required:"true"`
	DB                string        `yaml:"-" env:"POSTGRES_DBNAME" env-required:"true"`
	Username          string        `yaml:"-" env:"POSTGRES_USERNAME" env-required:"true"`
	Password          string        `yaml:"-" env:"POSTGRES_PASSWORD" env-required:"true"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout" env-default:"5s"`
}

type Telegram struct {
	ApiHash string `yaml:"-" env:"TELEGRAM_API_HASH" env-required:"true"`
	ApiID   int    `yaml:"-" env:"TELEGRAM_API_ID" env-required:"true"`
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
