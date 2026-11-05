package middlewares

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
)

const apiKeyHeader = "X-API-Key"

func NewAPIKeyAuth(apiKey string, log *slog.Logger) func(http.Handler) http.Handler {
	expected := []byte(apiKey)

	return func(next http.Handler) http.Handler {
		log = log.With(slog.String("component", "middlewares/api_auth"))
		log.Info("api key auth middleware is enabled")

		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			provided := req.Header.Get(apiKeyHeader)
			if provided == "" {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "missing api key")
				return
			}

			if subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
				writeAuthError(w, http.StatusForbidden, "forbidden", "invalid api key")
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiv1.Error{
		Code:    &code,
		Message: message,
	})
}
