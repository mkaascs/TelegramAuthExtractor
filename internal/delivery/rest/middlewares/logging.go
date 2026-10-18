package middlewares

import (
	"github.com/go-chi/chi/v5/middleware"
	"log/slog"
	"net/http"
	"time"
)

func NewLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		log = log.With(slog.String("component", "middlewares/logging"))
		log.Info("logging middleware is enabled")

		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			entry := log.With(slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.String("remote_addr", req.RemoteAddr),
				slog.String("user_agent", req.UserAgent()),
				slog.String("request_id", middleware.GetReqID(req.Context())),
			)

			now := time.Now()
			wrw := middleware.NewWrapResponseWriter(w, req.ProtoMajor)

			defer func() {
				entry.Info("request completed", slog.Int("status", wrw.Status()),
					slog.Int("bytes", wrw.BytesWritten()),
					slog.Duration("duration", time.Since(now)))
			}()

			next.ServeHTTP(wrw, req)
		})
	}
}
