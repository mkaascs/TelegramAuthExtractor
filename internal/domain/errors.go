package domain

import "errors"

var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrWrongState          = errors.New("session in wrong state for this action")
	ErrCodeTimeout         = errors.New("code wait timeout")
	ErrCodeInvalid         = errors.New("invalid confirmation code")
	ErrDeviceUnavailable   = errors.New("device unavailable")
	ErrTooManyCodeAttempts = errors.New("too many code attempts")
)
