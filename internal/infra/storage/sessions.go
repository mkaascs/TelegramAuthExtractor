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

const sessionColumns = `id, phone, status, error, code_attempts, created_at, expires_at`

type Session struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewSession(pool *pgxpool.Pool, log *slog.Logger) *Session {
	return &Session{pool: pool, log: log}
}

func (s *Session) CreateOrGetActive(ctx context.Context, session *domain.Session) (*domain.Session, bool, error) {
	const fn = "infra.storage.Session.CreateOrGetActive"

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", fn, err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var id uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO sessions(id, phone, status, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (phone) WHERE status NOT IN ('done', 'failed') DO NOTHING
		 RETURNING id`,
		uuid.UUID(session.ID), session.Phone, string(session.Status),
		session.CreatedAt, session.ExpiresAt).Scan(&id)

	switch {
	case err == nil:
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("%s: %w", fn, err)
		}

		return session, true, nil

	case errors.Is(err, pgx.ErrNoRows):
		existing, err := scanSession(tx.QueryRow(ctx,
			`SELECT `+sessionColumns+` FROM sessions
			 WHERE phone = $1 AND status NOT IN ('done', 'failed')
			 ORDER BY created_at DESC LIMIT 1`, session.Phone))

		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", fn, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("%s: %w", fn, err)
		}

		return existing, false, nil

	default:
		return nil, false, fmt.Errorf("%s: %w", fn, err)
	}
}

func (s *Session) Get(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error) {
	const fn = "infra.storage.Session.Get"

	session, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, sessionID))

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSessionNotFound
		}

		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return session, nil
}

func (s *Session) Update(ctx context.Context, session *domain.Session) error {
	const fn = "infra.storage.Session.Update"

	tag, err := s.pool.Exec(ctx,
		`UPDATE sessions SET status = $1, error = $2, code_attempts = $3 WHERE id = $4`,
		string(session.Status), session.Error, session.CodeAttempts, uuid.UUID(session.ID))

	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrSessionNotFound
	}

	return nil
}

func (s *Session) Complete(ctx context.Context, session *domain.Session, result *domain.ExtractedData) error {
	const fn = "infra.storage.Session.Complete"

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx,
		`INSERT INTO results(session_id, dc_id, auth_key, user_id) VALUES ($1, $2, $3, $4)`,
		uuid.UUID(session.ID), result.DCID, result.AuthKey, result.UserID); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE sessions SET status = $1 WHERE id = $2`,
		string(domain.StatusDone), uuid.UUID(session.ID))

	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", fn, domain.ErrSessionNotFound)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	session.Status = domain.StatusDone
	return nil
}

func (s *Session) Delete(ctx context.Context, sessionID uuid.UUID) error {
	const fn = "infra.storage.Session.Delete"

	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (s *Session) ClaimPending(ctx context.Context, limit int) ([]*domain.Session, error) {
	const fn = "infra.storage.Session.ClaimPending"

	rows, err := s.pool.Query(ctx,
		`UPDATE sessions SET status = $1
		 WHERE id IN (
		     SELECT id FROM sessions WHERE status = $2 ORDER BY created_at LIMIT $3
		 ) RETURNING `+sessionColumns,
		string(domain.StatusQueued), string(domain.StatusPending), limit)

	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return sessions, nil
}

func (s *Session) RecoverStale(ctx context.Context) error {
	const fn = "infra.storage.Session.RecoverStale"

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET status = $1 WHERE status = $2`,
		string(domain.StatusPending), string(domain.StatusQueued)); err != nil {

		return fmt.Errorf("%s: %w", fn, err)
	}

	message := domain.PublicErrMessage(domain.ErrServiceRestarted)
	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET status = $1, error = $2
		 WHERE status IN ('logging_in', 'waiting_for_code', 'entering_code', 'extracting', 'cleanup')`,
		string(domain.StatusFailed), message); err != nil {

		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner) (*domain.Session, error) {
	var (
		id      uuid.UUID
		status  string
		session domain.Session
	)

	if err := row.Scan(&id, &session.Phone, &status, &session.Error,
		&session.CodeAttempts, &session.CreatedAt, &session.ExpiresAt); err != nil {
		return nil, err
	}

	session.ID = domain.SessionID(id)
	session.Status = domain.Status(status)
	return &session, nil
}
