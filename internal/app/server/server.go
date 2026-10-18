package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"log"
	"log/slog"
	"net/http"
	"strings"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/delivery/rest/middlewares"
)

type Server struct {
	http *http.Server
}

func NewServer(handlers apiv1.ServerInterface, cfg config.Config, lg *slog.Logger) *Server {
	spec, err := apiv1.GetSpec()
	if err != nil {
		log.Fatal("failed to get spec")
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middlewares.NewLogger(lg))
	router.Use(nethttpmiddleware.OapiRequestValidatorWithOptions(spec,
		&nethttpmiddleware.Options{
			SilenceServersWarning: true,
			ErrorHandler:          validationErrorHandler,
		}))

	apiv1.HandlerFromMuxWithBaseURL(handlers, router, "/api/v1")

	return &Server{http: &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Http.Port),
		Handler:      router,
		WriteTimeout: cfg.Http.WriteTimeout,
		ReadTimeout:  cfg.Http.ReadTimeout,
	}}
}

func (s *Server) Run() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func validationErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	code := "bad_request"
	_ = json.NewEncoder(w).Encode(apiv1.Error{
		Code:    &code,
		Message: cleanValidationMessage(message),
	})
}

func cleanValidationMessage(raw string) string {
	if index := strings.Index(raw, `Error at "/`); index != -1 {
		rest := raw[index+len(`Error at "/`):]
		if j := strings.Index(rest, `"`); j != -1 {
			return "invalid field: " + rest[:j]
		}
	}

	return "invalid request body"
}
