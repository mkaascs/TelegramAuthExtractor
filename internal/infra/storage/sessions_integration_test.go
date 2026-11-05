//go:build integration
// +build integration

package storage_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"telegram-auth-extractor/internal/domain"
	"telegram-auth-extractor/internal/infra/storage"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("extractor_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)

	if err != nil {
		panic("start postgres container: " + err.Error())
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("connection string: " + err.Error())
	}

	if err := applyMigrations(dsn); err != nil {
		panic("apply migrations: " + err.Error())
	}

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		panic("open pool: " + err.Error())
	}

	code := m.Run()

	testPool.Close()
	_ = testcontainers.TerminateContainer(container)
	os.Exit(code)
}

func applyMigrations(dsn string) error {
	_, file, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")

	m, err := migrate.New("file://"+migrationsDir, dsn)
	if err != nil {
		return err
	}

	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE sessions, results RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func newSessionRepo() *storage.Session {
	return storage.NewSession(testPool, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func resultsCount(t *testing.T, sessionID uuid.UUID) int {
	t.Helper()
	var count int
	err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM results WHERE session_id = $1`, sessionID).Scan(&count)

	require.NoError(t, err)
	return count
}

func TestSession_CreateOrGetActive_Idempotency(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()
	const phone = "+79990000001"

	first := domain.NewSession(phone, time.Hour)
	stored, created, err := repo.CreateOrGetActive(ctx, first)
	require.NoError(t, err)
	require.True(t, created)

	got, created, err := repo.CreateOrGetActive(ctx, domain.NewSession(phone, time.Hour))
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, uuid.UUID(stored.ID), uuid.UUID(got.ID))

	require.NoError(t, repo.Complete(ctx, stored, &domain.EncryptedResult{DCID: 2, AuthKeyEncrypted: "sealed-k", UserID: 1}))

	_, created, err = repo.CreateOrGetActive(ctx, domain.NewSession(phone, time.Hour))
	require.NoError(t, err)
	require.True(t, created, "после done по тому же номеру создаётся новая активная сессия")
}

func TestSession_ClaimPending(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	for _, phone := range []string{"+79990000001", "+79990000002", "+79990000003"} {
		_, created, err := repo.CreateOrGetActive(ctx, domain.NewSession(phone, time.Hour))
		require.NoError(t, err)
		require.True(t, created)
	}

	claimed, err := repo.ClaimPending(ctx, 2)
	require.NoError(t, err)
	require.Len(t, claimed, 2, "claim ограничен лимитом")
	for _, s := range claimed {
		require.Equal(t, domain.StatusQueued, s.Status, "заклейменные становятся queued")
	}

	rest, err := repo.ClaimPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rest, 1, "остаётся одна pending; queued не возвращаются")

	empty, err := repo.ClaimPending(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, empty, "pending закончились")
}

func TestSession_RecoverStale(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	queued := domain.NewSession("+79990000001", time.Hour)
	_, _, err := repo.CreateOrGetActive(ctx, queued)
	require.NoError(t, err)
	claimed, err := repo.ClaimPending(ctx, 10) // -> queued
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	midflight := domain.NewSession("+79990000002", time.Hour)
	_, _, err = repo.CreateOrGetActive(ctx, midflight)
	require.NoError(t, err)
	midflight.Status = domain.StatusExtracting
	require.NoError(t, repo.Update(ctx, midflight))

	require.NoError(t, repo.RecoverStale(ctx))

	gotQueued, err := repo.Get(ctx, uuid.UUID(queued.ID))
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, gotQueued.Status, "queued возвращается в pending")

	gotMid, err := repo.Get(ctx, uuid.UUID(midflight.ID))
	require.NoError(t, err)
	require.Equal(t, domain.StatusFailed, gotMid.Status, "прерванная mid-flight падает в failed")
	require.NotNil(t, gotMid.Error)
}

func TestSession_Complete_And_DeleteCascade(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	s := domain.NewSession("+79990000001", time.Hour)
	_, _, err := repo.CreateOrGetActive(ctx, s)
	require.NoError(t, err)

	require.NoError(t, repo.Complete(ctx, s, &domain.EncryptedResult{DCID: 2, AuthKeyEncrypted: "sealed-key", UserID: 7}))

	done, err := repo.Get(ctx, uuid.UUID(s.ID))
	require.NoError(t, err)
	require.Equal(t, domain.StatusDone, done.Status)
	require.Equal(t, 1, resultsCount(t, uuid.UUID(s.ID)), "Complete записал результат")

	require.NoError(t, repo.Delete(ctx, uuid.UUID(s.ID)))
	require.Equal(t, 0, resultsCount(t, uuid.UUID(s.ID)), "удаление сессии каскадом снесло результат")
}
