package workers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
)

type DispatcherRepository interface {
	ClaimPending(ctx context.Context, limit int) ([]*domain.Session, error)
	RecoverStale(ctx context.Context) error
}

type Queue interface {
	Enqueue(session *domain.Session)
	FreeSlots() int
}

type Dispatcher struct {
	repo  DispatcherRepository
	queue Queue
	log   *slog.Logger
	cfg   config.Dispatcher
}

func NewDispatcher(repo DispatcherRepository, queue Queue, log *slog.Logger, cfg config.Dispatcher) *Dispatcher {
	return &Dispatcher{repo: repo, queue: queue, log: log, cfg: cfg}
}

func (d *Dispatcher) Recover(ctx context.Context) error {
	const fn = "services.workers.Dispatcher.Recover"

	if err := d.repo.RecoverStale(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	d.dispatch(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context) {
	const fn = "services.workers.Dispatcher.dispatch"
	log := d.log.With(slog.String("fn", fn))

	limit := d.queue.FreeSlots()
	if limit <= 0 {
		return
	}

	if d.cfg.BatchSize > 0 && d.cfg.BatchSize < limit {
		limit = d.cfg.BatchSize
	}

	sessions, err := d.repo.ClaimPending(ctx, limit)
	if err != nil {
		log.Error("failed to claim pending sessions", sloglib.Error(err))
		return
	}

	for _, session := range sessions {
		d.queue.Enqueue(session)
	}

	if len(sessions) > 0 {
		log.Info("dispatched pending sessions", slog.Int("count", len(sessions)))
	}
}
