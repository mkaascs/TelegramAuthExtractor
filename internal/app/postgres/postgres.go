package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging/sloglib"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool

	log *slog.Logger
	cfg config.Postgres
}

func NewDB(cfg config.Postgres, log *slog.Logger) (*DB, error) {
	connString := toConnectionString(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectionTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Error("failed to create pool", sloglib.Error(err))
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	return &DB{Pool: pool, log: log, cfg: cfg}, nil
}

func (db *DB) MustConnect() {
	if err := db.Connect(); err != nil {
		os.Exit(1)
	}
}

func (db *DB) Connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), db.cfg.ConnectionTimeout)
	defer cancel()

	if err := db.Pool.Ping(ctx); err != nil {
		db.log.Error("failed to ping database", sloglib.Error(err))
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if err := migrateDB(db.cfg, db.log); err != nil {
		db.log.Error("failed to migrate database", sloglib.Error(err))
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	db.log.Info("successfully connected and migrated to database", slog.String("driver", "postgresql"))
	return nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func toConnectionString(cfg config.Postgres) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Addr, cfg.DB)
}
