package storage

import (
	"context"
	"sync"
	"telegram-auth-extractor/internal/domain"

	"github.com/google/uuid"
)

type MemorySessionRepository struct {
	rwMx     sync.RWMutex
	sessions map[uuid.UUID]*domain.Session
}

func NewMemorySessionRepository() *MemorySessionRepository {
	return &MemorySessionRepository{
		sessions: make(map[uuid.UUID]*domain.Session),
	}
}

func (mr *MemorySessionRepository) Create(_ context.Context, session *domain.Session) error {
	mr.rwMx.Lock()
	defer mr.rwMx.Unlock()

	cp := *session
	mr.sessions[uuid.UUID(session.ID)] = &cp
	return nil
}

func (mr *MemorySessionRepository) Get(_ context.Context, sessionID uuid.UUID) (*domain.Session, error) {
	mr.rwMx.RLock()
	defer mr.rwMx.RUnlock()

	session, ok := mr.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}

	cp := *session
	return &cp, nil
}

func (mr *MemorySessionRepository) Delete(_ context.Context, sessionID uuid.UUID) error {
	mr.rwMx.Lock()
	defer mr.rwMx.Unlock()

	delete(mr.sessions, sessionID)
	return nil
}

func (mr *MemorySessionRepository) FindActiveByPhone(_ context.Context, phone string) (*domain.Session, error) {
	mr.rwMx.RLock()
	defer mr.rwMx.RUnlock()

	for _, session := range mr.sessions {
		if session.Phone == phone && !session.Status.IsTerminal() {
			cp := *session
			return &cp, nil
		}
	}

	return nil, nil
}

func (mr *MemorySessionRepository) Update(_ context.Context, session *domain.Session) error {
	mr.rwMx.Lock()
	defer mr.rwMx.Unlock()

	id := uuid.UUID(session.ID)
	if _, ok := mr.sessions[id]; !ok {
		return domain.ErrSessionNotFound
	}

	cp := *session
	mr.sessions[id] = &cp
	return nil
}
