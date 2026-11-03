package domain

import (
	"time"

	"github.com/google/uuid"
)

type SessionID uuid.UUID

type Session struct {
	ID           SessionID
	Phone        string
	Status       Status
	Result       *ExtractedData
	Error        *string
	CodeAttempts int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewSession(phone string) *Session {
	id, _ := uuid.NewV7()
	now := time.Now()

	return &Session{
		ID:        SessionID(id),
		Phone:     phone,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
