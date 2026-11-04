package workers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
	"telegram-auth-extractor/internal/lib/logging/sloglib"

	"github.com/google/uuid"
)

type SessionRepository interface {
	Get(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error)
	Update(ctx context.Context, session *domain.Session) error
	Complete(ctx context.Context, session *domain.Session, result *domain.ExtractedData) error
}

type Device interface {
	PrepareTelegram(ctx context.Context) error
	EnterPhone(ctx context.Context, phone string) error
	EnterCode(ctx context.Context, code string) error
	SessionFile(ctx context.Context) ([]byte, error)
	Clear(ctx context.Context) error
}

type Parser interface {
	Parse(ctx context.Context, data []byte) ([]domain.DatacenterKey, error)
}

type Verifier interface {
	Verify(ctx context.Context, keys []domain.DatacenterKey) (*domain.ExtractedData, error)
}

type waitingTask struct {
	id      uuid.UUID
	inputCh chan string
	cancel  context.CancelFunc
}

type Worker struct {
	sessions SessionRepository
	device   Device
	parser   Parser
	verifier Verifier

	log *slog.Logger
	cfg config.Worker

	mx      sync.Mutex
	queue   chan *domain.Session
	current *waitingTask
}

func NewWorker(sessions SessionRepository, device Device, parser Parser, verifier Verifier, log *slog.Logger, cfg config.Worker) *Worker {
	return &Worker{
		sessions: sessions,
		device:   device,
		parser:   parser,
		verifier: verifier,
		log:      log,
		cfg:      cfg,
		queue:    make(chan *domain.Session, cfg.QueueSize),
	}
}

func (w *Worker) Enqueue(session *domain.Session) {
	w.queue <- session
}

func (w *Worker) SubmitCode(sessionID uuid.UUID, code string) error {
	const fn = "services.workers.Worker.SubmitCode"

	w.mx.Lock()
	defer w.mx.Unlock()

	if w.current == nil || w.current.id != sessionID || w.current.inputCh == nil {
		return fmt.Errorf("%s: %w", fn, domain.ErrWrongState)
	}

	w.current.inputCh <- code
	w.current.inputCh = nil
	return nil
}

func (w *Worker) Cancel(sessionID uuid.UUID) error {
	const fn = "services.workers.Worker.Cancel"

	w.mx.Lock()
	defer w.mx.Unlock()

	if w.current == nil || w.current.id != sessionID {
		return fmt.Errorf("%s: %w", fn, domain.ErrSessionNotFound)
	}

	w.current.cancel()
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case session := <-w.queue:
			w.process(ctx, session)
		}
	}
}

func (w *Worker) process(ctx context.Context, session *domain.Session) {
	const fn = "services.workers.Worker.process"
	log := w.log.With(slog.String("fn", fn))

	sessionID := uuid.UUID(session.ID)

	if _, err := w.sessions.Get(ctx, sessionID); err != nil {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	task := &waitingTask{
		id:     sessionID,
		cancel: cancel,
	}

	w.setCurrent(task)
	defer w.setCurrent(nil)

	result, err := w.runPipeline(ctx, session)

	if err := w.runCleanup(session); err != nil {
		log.Error("failed to cleanup device", slog.String("session_id", sessionID.String()), sloglib.Error(err))
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("session canceled", slog.String("session_id", sessionID.String()))
			return
		}

		if err := w.setFailedStatus(ctx, session, err); err != nil {
			log.Error("failed to persist failed status", slog.String("session_id", sessionID.String()), sloglib.Error(err))
		}

		log.Warn("session failed", slog.String("session_id", sessionID.String()), sloglib.Error(err))
		return
	}

	if err := w.complete(ctx, session, result); err != nil {
		log.Error("failed to persist result", slog.String("session_id", sessionID.String()), sloglib.Error(err))

		if err := w.setFailedStatus(ctx, session, err); err != nil {
			log.Error("failed to persist failed status", slog.String("session_id", sessionID.String()), sloglib.Error(err))
		}

		return
	}

	log.Info("session extracted", slog.String("session_id", sessionID.String()))
}

func (w *Worker) complete(ctx context.Context, session *domain.Session, result *domain.ExtractedData) error {
	const fn = "services.workers.Worker.complete"

	if err := w.sessions.Complete(ctx, session, result); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (w *Worker) setCurrent(task *waitingTask) {
	w.mx.Lock()
	defer w.mx.Unlock()
	w.current = task
}

func (w *Worker) openInput(ch chan string) {
	w.mx.Lock()
	defer w.mx.Unlock()
	if w.current != nil {
		w.current.inputCh = ch
	}
}

func (w *Worker) closeInput() {
	w.mx.Lock()
	defer w.mx.Unlock()
	if w.current != nil {
		w.current.inputCh = nil
	}
}

func (w *Worker) setStatus(ctx context.Context, session *domain.Session, status domain.Status) error {
	const fn = "services.workers.Worker.setStatus"

	if status == domain.StatusFailed {
		return fmt.Errorf("%s: set failed status using Worker.setFailedStatus", fn)
	}

	session.Status = status
	if err := w.sessions.Update(ctx, session); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (w *Worker) setFailedStatus(ctx context.Context, session *domain.Session, err error) error {
	const fn = "services.workers.Worker.setFailedStatus"

	message := domain.PublicErrMessage(err)
	session.Status = domain.StatusFailed
	session.Error = &message
	if err := w.sessions.Update(ctx, session); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}
