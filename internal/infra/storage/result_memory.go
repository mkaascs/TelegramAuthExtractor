package storage

import (
	"context"
	"sync"
	"telegram-auth-extractor/internal/domain"

	"github.com/google/uuid"
)

type MemoryResultRepository struct {
	rwMx    sync.RWMutex
	results map[uuid.UUID]*domain.ExtractedData
}

func NewMemoryResultRepository() *MemoryResultRepository {
	return &MemoryResultRepository{
		results: make(map[uuid.UUID]*domain.ExtractedData),
	}
}

func (rr *MemoryResultRepository) Create(_ context.Context, sessionID uuid.UUID, data *domain.ExtractedData) error {
	rr.rwMx.Lock()
	defer rr.rwMx.Unlock()

	cp := *data
	rr.results[sessionID] = &cp
	return nil
}

func (rr *MemoryResultRepository) GetBySession(_ context.Context, sessionID uuid.UUID) (*domain.ExtractedData, error) {
	rr.rwMx.RLock()
	defer rr.rwMx.RUnlock()

	data, ok := rr.results[sessionID]
	if !ok {
		return nil, nil
	}

	cp := *data
	return &cp, nil
}
