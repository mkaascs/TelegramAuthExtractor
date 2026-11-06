package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"telegram-auth-extractor/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Result struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewResult(pool *pgxpool.Pool, log *slog.Logger) *Result {
	return &Result{pool: pool, log: log}
}

func (r *Result) GetBySession(ctx context.Context, sessionID uuid.UUID) (*domain.EncryptedResult, error) {
	const fn = "infra.storage.Result.GetBySession"

	var data domain.EncryptedResult
	err := r.pool.QueryRow(ctx,
		`SELECT dc_id, auth_key_encrypted, user_id FROM results WHERE session_id = $1 LIMIT 1`, sessionID).
		Scan(&data.DCID, &data.AuthKeyEncrypted, &data.UserID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return &data, nil
}
