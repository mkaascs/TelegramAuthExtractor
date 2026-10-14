package storage

import (
	"context"
	"github.com/google/uuid"
	"sync"
	"telegram-auth-extractor/internal/domain"
)

type MemoryRepository struct {
	rwMx     sync.RWMutex
	sessions map[uuid.UUID]*domain.Session
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		sessions: make(map[uuid.UUID]*domain.Session),
	}
}

func (mr *MemoryRepository) Create(_ context.Context, session *domain.Session) error {
	mr.rwMx.Lock()
	defer mr.rwMx.Unlock()

	mr.sessions[uuid.UUID(session.ID)] = session
	return nil
}

func (mr *MemoryRepository) Get(_ context.Context, sessionID uuid.UUID) (*domain.Session, error) {
	mr.rwMx.RLock()
	defer mr.rwMx.RUnlock()

	session, ok := mr.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}

	cp := *session
	return &cp, nil
}

func (mr *MemoryRepository) Delete(_ context.Context, sessionID uuid.UUID) error {
	mr.rwMx.Lock()
	defer mr.rwMx.Unlock()

	delete(mr.sessions, sessionID)
	return nil
}

func (mr *MemoryRepository) FindActiveByPhone(_ context.Context, phone string) (*domain.Session, error) {
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

func (mr *MemoryRepository) Update(_ context.Context, session *domain.Session) error {
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
