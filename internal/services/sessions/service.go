package sessions

import (
	"context"
	"errors"
	"fmt"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"

	"github.com/google/uuid"
)

type SessionRepository interface {
	CreateOrGetActive(ctx context.Context, session *domain.Session) (*domain.Session, bool, error)
	Get(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error)
	Delete(ctx context.Context, sessionID uuid.UUID) error
}

type ResultRepository interface {
	GetBySession(ctx context.Context, sessionID uuid.UUID) (*domain.ExtractedData, error)
}

type Processor interface {
	Enqueue(session *domain.Session)
	SubmitCode(sessionID uuid.UUID, code string) error
	Cancel(sessionID uuid.UUID) error
}

type Service struct {
	sessions  SessionRepository
	results   ResultRepository
	processor Processor
	cfg       config.Service
}

func NewService(sessions SessionRepository, results ResultRepository, processor Processor, cfg config.Service) *Service {
	return &Service{sessions: sessions, results: results, processor: processor, cfg: cfg}
}

func (s *Service) CreateSession(ctx context.Context, phone string) (*domain.Session, bool, error) {
	const fn = "services.sessions.Service.CreateSession"

	session := domain.NewSession(phone, s.cfg.SessionTTL)

	stored, created, err := s.sessions.CreateOrGetActive(ctx, session)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", fn, err)
	}

	if created {
		s.processor.Enqueue(stored)
	}

	return stored, created, nil
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

	if session.Status != domain.StatusDone {
		return nil, fmt.Errorf("%s: %w", fn, domain.ErrWrongState)
	}

	result, err := s.results.GetBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if result == nil {
		return nil, fmt.Errorf("%s: %w", fn, domain.ErrWrongState)
	}

	return result, nil
}
