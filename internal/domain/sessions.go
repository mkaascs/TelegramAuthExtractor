package domain

import (
	"github.com/google/uuid"
	"time"
)

type SessionID uuid.UUID

type Session struct {
	ID        SessionID
	Phone     string
	Status    Status
	Result    *ExtractedData
	Error     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}
