package postgres

import (
	"errors"
	"fmt"
	"log/slog"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging/sloglib"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func migrateDB(cfg config.Postgres, log *slog.Logger) error {
	migration, err := migrate.New("file://migrations", toConnectionString(cfg))
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	defer func(migration *migrate.Migrate) {
		if err, _ := migration.Close(); err != nil {
			log.Warn("failed to close migration", sloglib.Error(err))
		}
	}(migration)

	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to run migration: %w", err)
	}

	return nil
}
