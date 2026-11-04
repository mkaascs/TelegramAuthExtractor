package workers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
)

type mockDispatchRepo struct {
	claimFn    func(ctx context.Context, limit int) ([]*domain.Session, error)
	recovered  bool
	recoverErr error
}

func (m *mockDispatchRepo) ClaimPending(ctx context.Context, limit int) ([]*domain.Session, error) {
	return m.claimFn(ctx, limit)
}

func (m *mockDispatchRepo) RecoverStale(_ context.Context) error {
	m.recovered = true
	return m.recoverErr
}

type mockQueue struct {
	free     int
	enqueued []*domain.Session
}

func (q *mockQueue) Enqueue(s *domain.Session) {
	q.enqueued = append(q.enqueued, s)
	q.free--
}

func (q *mockQueue) FreeSlots() int {
	return q.free
}

func sessions(n int) []*domain.Session {
	out := make([]*domain.Session, n)
	for i := range out {
		out[i] = domain.NewSession("+79991234567", 0)
	}
	return out
}

func TestDispatcher_dispatch_ClaimsUpToFreeSlots(t *testing.T) {
	queue := &mockQueue{free: 3}
	var gotLimit int
	repo := &mockDispatchRepo{claimFn: func(_ context.Context, limit int) ([]*domain.Session, error) {
		gotLimit = limit
		return sessions(limit), nil
	}}

	d := NewDispatcher(repo, queue, discardLogger(), config.Dispatcher{BatchSize: 10})
	d.dispatch(context.Background())

	require.Equal(t, 3, gotLimit, "claim не больше свободных слотов")
	require.Len(t, queue.enqueued, 3)
}

func TestDispatcher_dispatch_RespectsBatchSize(t *testing.T) {
	queue := &mockQueue{free: 10}
	var gotLimit int
	repo := &mockDispatchRepo{claimFn: func(_ context.Context, limit int) ([]*domain.Session, error) {
		gotLimit = limit
		return sessions(limit), nil
	}}

	d := NewDispatcher(repo, queue, discardLogger(), config.Dispatcher{BatchSize: 4})
	d.dispatch(context.Background())

	require.Equal(t, 4, gotLimit, "claim ограничен batch size")
	require.Len(t, queue.enqueued, 4)
}

func TestDispatcher_dispatch_SkipsWhenFull(t *testing.T) {
	queue := &mockQueue{free: 0}
	called := false
	repo := &mockDispatchRepo{claimFn: func(_ context.Context, _ int) ([]*domain.Session, error) {
		called = true
		return nil, nil
	}}

	d := NewDispatcher(repo, queue, discardLogger(), config.Dispatcher{BatchSize: 10})
	d.dispatch(context.Background())

	require.False(t, called, "при полной очереди БД не трогаем")
	require.Empty(t, queue.enqueued)
}

func TestDispatcher_dispatch_ClaimError(t *testing.T) {
	queue := &mockQueue{free: 5}
	repo := &mockDispatchRepo{claimFn: func(_ context.Context, _ int) ([]*domain.Session, error) {
		return nil, errors.New("db down")
	}}

	d := NewDispatcher(repo, queue, discardLogger(), config.Dispatcher{BatchSize: 10})
	d.dispatch(context.Background())

	require.Empty(t, queue.enqueued, "ошибка claim не кладёт ничего в очередь")
}

func TestDispatcher_Recover(t *testing.T) {
	repo := &mockDispatchRepo{}
	d := NewDispatcher(repo, &mockQueue{}, discardLogger(), config.Dispatcher{})

	require.NoError(t, d.Recover(context.Background()))
	require.True(t, repo.recovered)
}
