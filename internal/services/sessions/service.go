package sessions

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"telegram-auth-extractor/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, session *domain.Session) error
	Get(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error)
	FindActiveByPhone(ctx context.Context, phone string) (*domain.Session, error)
	Delete(ctx context.Context, sessionID uuid.UUID) error
}

type Processor interface {
	Enqueue(session *domain.Session)
	SubmitCode(sessionID uuid.UUID, code string) error
	Cancel(sessionID uuid.UUID) error
}

type Service struct {
	sessions  Repository
	processor Processor
	createMx  sync.Mutex
}

func NewService(sessions Repository, processor Processor) *Service {
	return &Service{sessions: sessions, processor: processor}
}

func (s *Service) CreateSession(ctx context.Context, phone string) (*domain.Session, bool, error) {
	const fn = "services.sessions.Service.CreateSession"

	s.createMx.Lock()
	defer s.createMx.Unlock()

	existing, err := s.sessions.FindActiveByPhone(ctx, phone)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", fn, err)
	}

	if existing != nil {
		return existing, false, nil
	}

	session := domain.NewSession(phone)
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, false, fmt.Errorf("%s: %w", fn, err)
	}

	s.processor.Enqueue(session)
	return session, true, nil
}

func (s *Service) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	const fn = "services.sessions.Service.DeleteSession"

	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := s.processor.Cancel(sessionID); err != nil && !errors.Is(err, domain.ErrSessionNotFound) {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (s *Service) GetSession(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error) {
	const fn = "services.sessions.Service.GetSession"

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return session, nil
}

func (s *Service) SubmitCode(ctx context.Context, sessionID uuid.UUID, code string) (*domain.Session, error) {
	const fn = "services.sessions.Service.SubmitCode"

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if err := s.processor.SubmitCode(sessionID, code); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return session, nil
}

func (s *Service) GetSessionResult(ctx context.Context, sessionID uuid.UUID) (*domain.ExtractedData, error) {
	const fn = "services.sessions.Service.GetSessionResult"

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if session.Status != domain.StatusDone || session.Result == nil {
		return nil, fmt.Errorf("%s: %w", fn, domain.ErrWrongState)
	}

	return session.Result, nil
}
