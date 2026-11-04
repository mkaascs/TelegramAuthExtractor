package sessions

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
)

func idOf(s *domain.Session) uuid.UUID { return uuid.UUID(s.ID) }

type mockRepo struct {
	createOrGetFn func(ctx context.Context, s *domain.Session) (*domain.Session, bool, error)
	getFn         func(ctx context.Context, id uuid.UUID) (*domain.Session, error)
	deleteFn      func(ctx context.Context, id uuid.UUID) error
}

func (m mockRepo) CreateOrGetActive(ctx context.Context, s *domain.Session) (*domain.Session, bool, error) {
	return m.createOrGetFn(ctx, s)
}

func (m mockRepo) Get(ctx context.Context, id uuid.UUID) (*domain.Session, error) {
	return m.getFn(ctx, id)
}

func (m mockRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

type mockResultRepo struct {
	getFn func(ctx context.Context, id uuid.UUID) (*domain.ExtractedData, error)
}

func (m mockResultRepo) GetBySession(ctx context.Context, id uuid.UUID) (*domain.ExtractedData, error) {
	if m.getFn == nil {
		return nil, nil
	}
	return m.getFn(ctx, id)
}

type mockProcessor struct {
	enqueueFn    func(s *domain.Session)
	submitCodeFn func(id uuid.UUID, code string) error
	cancelFn     func(id uuid.UUID) error
}

func (m mockProcessor) Enqueue(s *domain.Session) {
	m.enqueueFn(s)
}

func (m mockProcessor) SubmitCode(id uuid.UUID, c string) error {
	return m.submitCodeFn(id, c)
}

func (m mockProcessor) Cancel(id uuid.UUID) error {
	return m.cancelFn(id)
}

func TestService_CreateSession(t *testing.T) {
	ctx := context.Background()
	const phone = "+79991234567"

	t.Run("new phone -> created, enqueued", func(t *testing.T) {
		var enqueued bool
		svc := NewService(
			mockRepo{
				createOrGetFn: func(_ context.Context, s *domain.Session) (*domain.Session, bool, error) {
					return s, true, nil
				},
			},
			mockResultRepo{},
			mockProcessor{
				enqueueFn: func(_ *domain.Session) { enqueued = true },
			},
			config.Service{},
		)

		session, created, err := svc.CreateSession(ctx, phone)
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, phone, session.Phone)
		require.True(t, enqueued, "новая задача должна попасть в очередь")
	})

	t.Run("active exists -> idempotent, not enqueued", func(t *testing.T) {
		existing := domain.NewSession(phone, 0)
		svc := NewService(
			mockRepo{
				createOrGetFn: func(_ context.Context, _ *domain.Session) (*domain.Session, bool, error) {
					return existing, false, nil
				},
			},
			mockResultRepo{},
			mockProcessor{
				enqueueFn: func(_ *domain.Session) {
					t.Fatal("enqueue не должен вызываться для существующей задачи")
				},
			},
			config.Service{},
		)

		session, created, err := svc.CreateSession(ctx, phone)
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, idOf(existing), idOf(session))
	})

	t.Run("repo error -> propagated, not enqueued", func(t *testing.T) {
		wantErr := errors.New("insert failed")
		svc := NewService(
			mockRepo{
				createOrGetFn: func(_ context.Context, _ *domain.Session) (*domain.Session, bool, error) {
					return nil, false, wantErr
				},
			},
			mockResultRepo{},
			mockProcessor{
				enqueueFn: func(_ *domain.Session) { t.Fatal("enqueue не должен вызываться при ошибке") },
			},
			config.Service{},
		)

		_, _, err := svc.CreateSession(ctx, phone)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestService_GetSession(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		got, err := svc.GetSession(ctx, idOf(s))
		require.NoError(t, err)
		require.Equal(t, idOf(s), idOf(got))
	})

	t.Run("not found propagated", func(t *testing.T) {
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
				return nil, domain.ErrSessionNotFound
			}},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		_, err := svc.GetSession(ctx, uuid.New())
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

func TestService_GetSessionResult(t *testing.T) {
	ctx := context.Background()

	t.Run("done -> returns result", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		s.Status = domain.StatusDone
		result := &domain.ExtractedData{DCID: 2, AuthKey: []byte("key"), UserID: 1153367122}

		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.ExtractedData, error) {
				return result, nil
			}},
			mockProcessor{},
			config.Service{},
		)

		res, err := svc.GetSessionResult(ctx, idOf(s))
		require.NoError(t, err)
		require.Equal(t, 2, res.DCID)
		require.Equal(t, int64(1153367122), res.UserID)
	})

	t.Run("not done -> ErrWrongState", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		s.Status = domain.StatusExtracting

		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		_, err := svc.GetSessionResult(ctx, idOf(s))
		require.ErrorIs(t, err, domain.ErrWrongState)
	})

	t.Run("done but result missing -> ErrWrongState", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		s.Status = domain.StatusDone

		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.ExtractedData, error) {
				return nil, nil
			}},
			mockProcessor{},
			config.Service{},
		)

		_, err := svc.GetSessionResult(ctx, idOf(s))
		require.ErrorIs(t, err, domain.ErrWrongState)
	})

	t.Run("not found propagated", func(t *testing.T) {
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
				return nil, domain.ErrSessionNotFound
			}},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		_, err := svc.GetSessionResult(ctx, uuid.New())
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

func TestService_SubmitCode(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		var called bool
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{},
			mockProcessor{submitCodeFn: func(_ uuid.UUID, _ string) error { called = true; return nil }},
			config.Service{},
		)

		got, err := svc.SubmitCode(ctx, idOf(s), "12345")
		require.NoError(t, err)
		require.Equal(t, idOf(s), idOf(got))
		require.True(t, called)
	})

	t.Run("not found -> processor not called", func(t *testing.T) {
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
				return nil, domain.ErrSessionNotFound
			}},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		_, err := svc.SubmitCode(ctx, uuid.New(), "12345")
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})

	t.Run("wrong state propagated", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil }},
			mockResultRepo{},
			mockProcessor{submitCodeFn: func(_ uuid.UUID, _ string) error { return domain.ErrWrongState }},
			config.Service{},
		)

		_, err := svc.SubmitCode(ctx, idOf(s), "12345")
		require.ErrorIs(t, err, domain.ErrWrongState)
	})
}

func TestService_DeleteSession(t *testing.T) {
	ctx := context.Background()

	t.Run("active -> cancel + delete", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		var cancelled, deleted bool
		svc := NewService(
			mockRepo{
				getFn:    func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil },
				deleteFn: func(_ context.Context, _ uuid.UUID) error { deleted = true; return nil },
			},
			mockResultRepo{},
			mockProcessor{cancelFn: func(_ uuid.UUID) error { cancelled = true; return nil }},
			config.Service{},
		)

		require.NoError(t, svc.DeleteSession(ctx, idOf(s)))
		require.True(t, cancelled)
		require.True(t, deleted)
	})

	t.Run("not found -> no cancel, no delete", func(t *testing.T) {
		svc := NewService(
			mockRepo{getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
				return nil, domain.ErrSessionNotFound
			}},
			mockResultRepo{},
			mockProcessor{},
			config.Service{},
		)

		err := svc.DeleteSession(ctx, uuid.New())
		require.ErrorIs(t, err, domain.ErrSessionNotFound)
	})

	t.Run("nothing to cancel is ignored, still deletes", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		var deleted bool
		svc := NewService(
			mockRepo{
				getFn:    func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil },
				deleteFn: func(_ context.Context, _ uuid.UUID) error { deleted = true; return nil },
			},
			mockResultRepo{},
			mockProcessor{cancelFn: func(_ uuid.UUID) error { return domain.ErrSessionNotFound }},
			config.Service{},
		)

		require.NoError(t, svc.DeleteSession(ctx, idOf(s)))
		require.True(t, deleted, "запись удаляется, даже если отменять нечего")
	})

	t.Run("cancel hard error -> propagated, no delete", func(t *testing.T) {
		s := domain.NewSession("+79991234567", 0)
		wantErr := errors.New("adb cleanup failed")
		svc := NewService(
			mockRepo{
				getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) { return s, nil },
				deleteFn: func(_ context.Context, _ uuid.UUID) error {
					t.Fatal("delete не должен вызываться")
					return nil
				},
			},
			mockResultRepo{},
			mockProcessor{cancelFn: func(_ uuid.UUID) error { return wantErr }},
			config.Service{},
		)

		err := svc.DeleteSession(ctx, idOf(s))
		require.ErrorIs(t, err, wantErr)
	})
}

type stubRepo struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*domain.Session
}

func newStubRepo() *stubRepo {
	return &stubRepo{sessions: make(map[uuid.UUID]*domain.Session)}
}

func (r *stubRepo) CreateOrGetActive(_ context.Context, s *domain.Session) (*domain.Session, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.sessions {
		if existing.Phone == s.Phone && existing.Status != domain.StatusDone && existing.Status != domain.StatusFailed {
			return existing, false, nil
		}
	}

	r.sessions[uuid.UUID(s.ID)] = s
	return s, true, nil
}

func (r *stubRepo) Get(_ context.Context, id uuid.UUID) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return s, nil
}

func (r *stubRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

func TestService_CreateSession_ConcurrentDedup(t *testing.T) {
	ctx := context.Background()

	var enqueued atomic.Int64
	svc := NewService(newStubRepo(), mockResultRepo{}, mockProcessor{
		enqueueFn: func(_ *domain.Session) { enqueued.Add(1) },
	}, config.Service{})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = svc.CreateSession(ctx, "+79991234567")
		}()
	}

	wg.Wait()

	require.Equal(t, int64(1), enqueued.Load(),
		"на один номер должна создаться ровно одна задача, несмотря на гонку")
}
