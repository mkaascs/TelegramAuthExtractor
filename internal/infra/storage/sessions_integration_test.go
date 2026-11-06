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

func newResultRepo() *storage.Result {
	return storage.NewResult(testPool, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestSession_Get_NotFound(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()

	_, err := repo.Get(context.Background(), uuid.New())
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestSession_Update_PersistsFields(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	s := domain.NewSession("+79990000001", time.Hour)
	_, _, err := repo.CreateOrGetActive(ctx, s)
	require.NoError(t, err)

	message := "code_timeout"
	s.Status = domain.StatusWaitingForCode
	s.Error = &message
	s.CodeAttempts = 3
	require.NoError(t, repo.Update(ctx, s))

	got, err := repo.Get(ctx, uuid.UUID(s.ID))
	require.NoError(t, err)
	require.Equal(t, domain.StatusWaitingForCode, got.Status)
	require.NotNil(t, got.Error)
	require.Equal(t, message, *got.Error)
	require.Equal(t, 3, got.CodeAttempts)
}

func TestSession_Update_NotFound(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()

	ghost := domain.NewSession("+79990000009", time.Hour) // не сохранена
	err := repo.Update(context.Background(), ghost)
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestSession_Delete_Idempotent(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()

	require.NoError(t, repo.Delete(context.Background(), uuid.New()),
		"удаление несуществующей сессии не ошибка")
}

func TestSession_Recipient_RoundTrip(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	const recipient = "age1qqpqtest-recipient-public-key"
	s := domain.NewSession("+79990000001", time.Hour)
	s.Recipient = recipient
	_, _, err := repo.CreateOrGetActive(ctx, s)
	require.NoError(t, err)

	got, err := repo.Get(ctx, uuid.UUID(s.ID))
	require.NoError(t, err)
	require.Equal(t, recipient, got.Recipient, "recipient сохраняется и читается")

	claimed, err := repo.ClaimPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, recipient, claimed[0].Recipient, "recipient возвращается и в ClaimPending")
}

func TestSession_Complete_MissingSession_RollsBack(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	ghost := domain.NewSession("+79990000009", time.Hour) // не сохранена
	err := repo.Complete(ctx, ghost, &domain.EncryptedResult{DCID: 1, AuthKeyEncrypted: "x", UserID: 1})
	require.Error(t, err, "Complete по несуществующей сессии падает")
	require.Equal(t, 0, resultsCount(t, uuid.UUID(ghost.ID)), "транзакция откатилась: результат не записан")
}

func TestResult_GetBySession(t *testing.T) {
	resetDB(t)
	sessions := newSessionRepo()
	results := newResultRepo()
	ctx := context.Background()

	s := domain.NewSession("+79990000001", time.Hour)
	_, _, err := sessions.CreateOrGetActive(ctx, s)
	require.NoError(t, err)

	want := &domain.EncryptedResult{
		DCID:             4,
		AuthKeyEncrypted: "-----BEGIN AGE ENCRYPTED FILE-----\n...\n-----END AGE ENCRYPTED FILE-----",
		UserID:           1153367122,
	}
	require.NoError(t, sessions.Complete(ctx, s, want))

	got, err := results.GetBySession(ctx, uuid.UUID(s.ID))
	require.NoError(t, err)
	require.Equal(t, want, got, "результат читается ровно как записан")
}

func TestResult_GetBySession_None(t *testing.T) {
	resetDB(t)
	results := newResultRepo()

	got, err := results.GetBySession(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Nil(t, got, "нет результата -> (nil, nil)")
}

func TestSession_DeleteExpired(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	expired := domain.NewSession("+79990000001", -time.Hour) // expires_at в прошлом
	_, _, err := repo.CreateOrGetActive(ctx, expired)
	require.NoError(t, err)

	fresh := domain.NewSession("+79990000002", time.Hour)
	_, _, err = repo.CreateOrGetActive(ctx, fresh)
	require.NoError(t, err)

	deleted, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "удаляется только истёкшая сессия")

	_, err = repo.Get(ctx, uuid.UUID(expired.ID))
	require.ErrorIs(t, err, domain.ErrSessionNotFound, "истёкшая удалена")

	_, err = repo.Get(ctx, uuid.UUID(fresh.ID))
	require.NoError(t, err, "не истёкшая осталась")
}

func TestSession_DeleteExpired_None(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()

	deleted, err := repo.DeleteExpired(context.Background())
	require.NoError(t, err)
	require.Zero(t, deleted, "нечего удалять")
}

func TestSession_DeleteExpired_CascadesResults(t *testing.T) {
	resetDB(t)
	repo := newSessionRepo()
	ctx := context.Background()

	s := domain.NewSession("+79990000001", -time.Hour)
	_, _, err := repo.CreateOrGetActive(ctx, s)
	require.NoError(t, err)
	require.NoError(t, repo.Complete(ctx, s, &domain.EncryptedResult{DCID: 1, AuthKeyEncrypted: "x", UserID: 1}))
	require.Equal(t, 1, resultsCount(t, uuid.UUID(s.ID)))

	deleted, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.Equal(t, 0, resultsCount(t, uuid.UUID(s.ID)), "результат истёкшей сессии снесён каскадом")
}
