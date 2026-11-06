package workers

import (
	"context"
	"log/slog"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
	"time"
)

type ReaperRepository interface {
	DeleteExpired(ctx context.Context) (int, error)
}

type Reaper struct {
	repo ReaperRepository
	log  *slog.Logger
	cfg  config.Reaper
}

func NewReaper(repo ReaperRepository, log *slog.Logger, cfg config.Reaper) *Reaper {
	return &Reaper{
		repo: repo,
		log:  log,
		cfg:  cfg,
	}
}

func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reap(ctx)
		}
	}
}

func (r *Reaper) reap(ctx context.Context) {
	const fn = "services.workers.Reaper.reap"
	log := r.log.With(slog.String("fn", fn))

	deleted, err := r.repo.DeleteExpired(ctx)
	if err != nil {
		log.Error("failed to delete expired sessions", sloglib.Error(err))
		return
	}

	if deleted > 0 {
		log.Info("deleted expired sessions", slog.Int("deleted", deleted))
		return
	}

	log.Debug("there are no expired sessions")
}
