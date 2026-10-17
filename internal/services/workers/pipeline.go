package workers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"telegram-auth-extractor/internal/domain"
)

const maxCodeAttempts = 3

func (w *Worker) runPipeline(ctx context.Context, session *domain.Session) error {
	const fn = "services.workers.Worker.runPipeline"

	if err := w.login(ctx, session); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	for {
		code, err := w.waitForCode(ctx, session)
		if err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}

		err = w.enterCode(ctx, session, code)
		if err == nil {
			break
		}

		if errors.Is(err, domain.ErrCodeInvalid) {
			session.CodeAttempts++
			if session.CodeAttempts >= maxCodeAttempts {
				return fmt.Errorf("%s: %w", fn, domain.ErrTooManyCodeAttempts)
			}

			w.log.Info("wrong code, awaiting retry",
				slog.Int("attempt", session.CodeAttempts),
				slog.String("session_id", uuid.UUID(session.ID).String()))
			continue
		}

		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := w.extract(ctx, session); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (w *Worker) login(ctx context.Context, session *domain.Session) error {
	const fn = "services.workers.Worker.login"

	if err := w.setStatus(ctx, session, domain.StatusLoggingIn); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := w.device.PrepareTelegram(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := w.device.EnterPhone(ctx, session.Phone); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (w *Worker) waitForCode(ctx context.Context, session *domain.Session) (string, error) {
	const fn = "services.workers.Worker.waitForCode"

	if err := w.setStatus(ctx, session, domain.StatusWaitingForCode); err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	ch := make(chan string, 1)
	w.openInput(ch)
	defer w.closeInput()

	select {
	case code := <-ch:
		return code, nil
	case <-time.After(w.cfg.CodeWait):
		return "", fmt.Errorf("%s: %w", fn, domain.ErrCodeTimeout)
	case <-ctx.Done():
		return "", fmt.Errorf("%s: %w", fn, ctx.Err())
	}
}

func (w *Worker) enterCode(ctx context.Context, session *domain.Session, code string) error {
	const fn = "services.workers.Worker.enterCode"

	if err := w.setStatus(ctx, session, domain.StatusEnteringCode); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := w.device.EnterCode(ctx, code); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (w *Worker) extract(ctx context.Context, session *domain.Session) error {
	const fn = "services.workers.Worker.extract"

	if err := w.setStatus(ctx, session, domain.StatusExtracting); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	data, err := w.device.SessionFile(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	keys, err := w.parser.Parse(ctx, data)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	result, err := w.verifier.Verify(ctx, keys)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	session.Result = result
	return nil
}

func (w *Worker) runCleanup(session *domain.Session) error {
	const fn = "services.workers.Worker.runCleanup"

	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.CleanupTimeout)
	defer cancel()

	if err := w.setStatus(ctx, session, domain.StatusCleanup); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := w.device.Clear(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}
