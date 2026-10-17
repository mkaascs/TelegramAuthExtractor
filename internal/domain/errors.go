package domain

import "errors"

var (
	// sync
	ErrSessionNotFound = errors.New("session not found")
	ErrWrongState      = errors.New("session in wrong state for this action")

	// async
	ErrCodeTimeout         = errors.New("code wait timeout")
	ErrCodeInvalid         = errors.New("invalid confirmation code")
	ErrDeviceUnavailable   = errors.New("device unavailable")
	ErrTooManyCodeAttempts = errors.New("too many code attempts")
	ErrNoAuthKeys          = errors.New("no auth keys found")
	ErrVerificationFailed  = errors.New("verification failed")
	ErrScreenTimeout       = errors.New("screen wait timeout")
)

func PublicErrMessage(err error) string {
	switch {
	case errors.Is(err, ErrCodeTimeout):
		return "confirmation code was not provided in time"
	case errors.Is(err, ErrTooManyCodeAttempts):
		return "too many wrong code attempts"
	case errors.Is(err, ErrNoAuthKeys):
		return "no authorization data found on device"
	case errors.Is(err, ErrVerificationFailed):
		return "failed to verify extracted session"
	case errors.Is(err, ErrDeviceUnavailable):
		return "virtual device is unavailable"
	case errors.Is(err, ErrScreenTimeout):
		return "screen timeout. try again"
	default:
		return "internal error"
	}
}
