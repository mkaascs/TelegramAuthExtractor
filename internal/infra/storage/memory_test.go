package storage

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"telegram-auth-extractor/internal/domain"
	"testing"
)

func idOf(s *domain.Session) uuid.UUID { return uuid.UUID(s.ID) }

func TestMemoryRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))

	got, err := repo.Get(ctx, idOf(s))
	require.NoError(t, err)
	require.Equal(t, s.Phone, got.Phone)
	require.Equal(t, s.Status, got.Status)
}

func TestMemoryRepository_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	_, err := repo.Get(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestMemoryRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))
	require.NoError(t, repo.Delete(ctx, idOf(s)))

	_, err := repo.Get(ctx, idOf(s))
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestMemoryRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))

	s.Status = domain.StatusExtracting
	require.NoError(t, repo.Update(ctx, s))

	got, err := repo.Get(ctx, idOf(s))
	require.NoError(t, err)
	require.Equal(t, domain.StatusExtracting, got.Status)
}

func TestMemoryRepository_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567") // не сохранён
	err := repo.Update(ctx, s)
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestMemoryRepository_FindActiveByPhone(t *testing.T) {
	ctx := context.Background()
	const phone = "+79991234567"

	t.Run("none returns nil,nil", func(t *testing.T) {
		repo := NewMemoryRepository()
		got, err := repo.FindActiveByPhone(ctx, phone)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("active session is found", func(t *testing.T) {
		repo := NewMemoryRepository()
		s := domain.NewSession(phone)
		require.NoError(t, repo.Create(ctx, s))

		got, err := repo.FindActiveByPhone(ctx, phone)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, idOf(s), idOf(got))
	})

	t.Run("terminal session is ignored", func(t *testing.T) {
		repo := NewMemoryRepository()
		s := domain.NewSession(phone)
		s.Status = domain.StatusDone
		require.NoError(t, repo.Create(ctx, s))

		got, err := repo.FindActiveByPhone(ctx, phone)
		require.NoError(t, err)
		require.Nil(t, got, "завершённая сессия не должна считаться активной")
	})

	t.Run("matches by exact phone", func(t *testing.T) {
		repo := NewMemoryRepository()
		a := domain.NewSession("+70000000001")
		b := domain.NewSession("+70000000002")
		require.NoError(t, repo.Create(ctx, a))
		require.NoError(t, repo.Create(ctx, b))

		got, err := repo.FindActiveByPhone(ctx, "+70000000002")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, idOf(b), idOf(got))
	})
}

func TestMemoryRepository_Get_ReturnsCopy(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))

	got, err := repo.Get(ctx, idOf(s))
	require.NoError(t, err)
	got.Status = domain.StatusFailed

	again, err := repo.Get(ctx, idOf(s))
	require.NoError(t, err)
	require.NotEqual(t, domain.StatusFailed, again.Status,
		"мутация результата Get не должна менять хранимую сессию")
}

func TestMemoryRepository_Create_StoresCopy(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))

	s.Status = domain.StatusFailed

	got, err := repo.Get(ctx, idOf(s))
	require.NoError(t, err)
	require.NotEqual(t, domain.StatusFailed, got.Status,
		"мутация объекта после Create не должна менять хранимую сессию")
}

func TestMemoryRepository_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()

	s := domain.NewSession("+79991234567")
	require.NoError(t, repo.Create(ctx, s))
	id := idOf(s)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			upd := domain.NewSession("+79991234567")
			upd.ID = s.ID
			upd.Status = domain.StatusExtracting
			_ = repo.Update(ctx, upd)
		}()

		go func() {
			defer wg.Done()
			if got, err := repo.Get(ctx, id); err == nil {
				got.Status = domain.StatusExtracting
			}
		}()

		go func() {
			defer wg.Done()
			if got, err := repo.Get(ctx, id); err == nil {
				_ = got.Status
			}
		}()
	}

	wg.Wait()
}
