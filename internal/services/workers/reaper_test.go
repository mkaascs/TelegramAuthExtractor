package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/config"
)

type mockReaperRepo struct {
	deleteFn func(ctx context.Context) (int, error)
}

func (m *mockReaperRepo) DeleteExpired(ctx context.Context) (int, error) {
	return m.deleteFn(ctx)
}

func TestReaper_reap_CallsDeleteExpired(t *testing.T) {
	var called bool
	repo := &mockReaperRepo{deleteFn: func(context.Context) (int, error) {
		called = true
		return 3, nil
	}}

	r := NewReaper(repo, discardLogger(), config.Reaper{})
	r.reap(context.Background())

	require.True(t, called, "reap дёргает DeleteExpired")
}

func TestReaper_reap_SwallowsError(t *testing.T) {
	repo := &mockReaperRepo{deleteFn: func(context.Context) (int, error) {
		return 0, errors.New("db down")
	}}

	r := NewReaper(repo, discardLogger(), config.Reaper{})

	require.NotPanics(t, func() { r.reap(context.Background()) },
		"ошибка удаления логируется, но не роняет reaper")
}

func TestReaper_Run_StopsOnContextCancel(t *testing.T) {
	repo := &mockReaperRepo{deleteFn: func(context.Context) (int, error) { return 0, nil }}
	r := NewReaper(repo, discardLogger(), config.Reaper{Interval: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run не завершился по отмене контекста")
	}
}

func TestReaper_Run_ReapsRepeatedly(t *testing.T) {
	calls := make(chan struct{}, 8)
	repo := &mockReaperRepo{deleteFn: func(context.Context) (int, error) {
		calls <- struct{}{}
		return 0, nil
	}}

	r := NewReaper(repo, discardLogger(), config.Reaper{Interval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	for i := 0; i < 3; i++ {
		select {
		case <-calls:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("reaper выполнил только %d проход(ов); ожидался периодический запуск по тикеру", i)
		}
	}
}
