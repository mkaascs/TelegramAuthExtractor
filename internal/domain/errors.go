package domain

import "errors"

var (
	// sync
	ErrSessionNotFound = errors.New("session not found")
	ErrWrongState      = errors.New("session in wrong state for this action")

	// async
	ErrCodeTimeout         = errors.New("code wait timeout")
	ErrCodeInvalid         = errors.New("invalid confirmation code")
	ErrTooManyCodeAttempts = errors.New("too many code attempts")
	ErrPhoneInvalid        = errors.New("invalid phone number")
	ErrTwoFANotSupported   = errors.New("two-factor authentication is not supported")
	ErrNoAuthKeys          = errors.New("no auth keys found")
	ErrVerificationFailed  = errors.New("verification failed")
	ErrScreenTimeout       = errors.New("screen wait timeout")
	ErrServiceRestarted    = errors.New("interrupted by service restart")
)

func PublicErrMessage(err error) string {
	switch {
	case errors.Is(err, ErrCodeTimeout):
		return "confirmation code was not provided in time"
	case errors.Is(err, ErrTooManyCodeAttempts):
		return "too many wrong code attempts"
	case errors.Is(err, ErrPhoneInvalid):
		return "invalid phone number"
	case errors.Is(err, ErrTwoFANotSupported):
		return "2fa is not supported"
	case errors.Is(err, ErrNoAuthKeys):
		return "no authorization data found on device"
	case errors.Is(err, ErrVerificationFailed):
		return "failed to verify extracted session"
	case errors.Is(err, ErrScreenTimeout):
		return "screen timeout. try again"
	case errors.Is(err, ErrServiceRestarted):
		return "session was interrupted by a service restart"
	default:
		return "internal error"
	}
}
