package middlewares

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"telegram-auth-extractor/internal/domain"
)

const bearerPrefix = "Bearer "

type sessionIDKey struct{}

type Verifier interface {
	Verify(raw string) (domain.SessionID, error)
}

func NewSessionAuth(verifier Verifier, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		log = log.With(slog.String("component", "middlewares/session_auth"))
		log.Info("session auth middleware is enabled")

		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			raw, ok := strings.CutPrefix(req.Header.Get("Authorization"), bearerPrefix)
			if !ok || raw == "" {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "missing session token")
				return
			}

			id, err := verifier.Verify(raw)
			if err != nil {
				writeAuthError(w, http.StatusForbidden, "forbidden", "invalid session token")
				return
			}

			next.ServeHTTP(w, req.WithContext(WithSessionID(req.Context(), id)))
		})
	}
}

func WithSessionID(ctx context.Context, id domain.SessionID) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

func GetSessionID(ctx context.Context) (domain.SessionID, bool) {
	id, ok := ctx.Value(sessionIDKey{}).(domain.SessionID)
	return id, ok
}
