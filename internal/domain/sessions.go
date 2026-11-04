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
	Error        *string
	CodeAttempts int
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func NewSession(phone string, ttl time.Duration) *Session {
	id, _ := uuid.NewV7()
	now := time.Now()

	return &Session{
		ID:        SessionID(id),
		Phone:     phone,
		Status:    StatusPending,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
}
