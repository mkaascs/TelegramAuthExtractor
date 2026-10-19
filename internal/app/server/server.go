package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/swaggest/swgui/v5emb"
	"log"
	"log/slog"
	"net/http"
	"strings"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/delivery/rest/middlewares"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
)

type Server struct {
	http *http.Server
	log  *slog.Logger
}

func NewServer(handlers apiv1.ServerInterface, cfg config.Config, lg *slog.Logger) *Server {
	spec, err := apiv1.GetSpec()
	if err != nil {
		log.Fatal("failed to get spec")
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middlewares.NewLogger(lg))

	if cfg.Env != config.EnvProd {
		router.Get("/swagger/openapi.json", newSpecHandler(lg))

		router.Mount("/swagger", v5emb.New(
			"Telegram Auth Extractor",
			"/swagger/openapi.json",
			"/swagger/"))
	}

	router.Route("/api/v1", func(r chi.Router) {
		r.Use(nethttpmiddleware.OapiRequestValidatorWithOptions(spec,
			&nethttpmiddleware.Options{
				SilenceServersWarning: true,
				ErrorHandler:          validationErrorHandler,
			}))

		apiv1.HandlerFromMux(handlers, r)
	})

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

func newSpecHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spec, err := apiv1.GetSpec()
		if err != nil {
			log.Warn("failed to get spec", sloglib.Error(err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spec)
	}
}

func validationErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	code := "error"
	if statusCode == http.StatusBadRequest {
		code = "bad_request"
		message = cleanValidationMessage(message)
	} else if statusCode == http.StatusNotFound {
		code = "not_found"
		message = "resource not found"
	}

	_ = json.NewEncoder(w).Encode(apiv1.Error{
		Code:    &code,
		Message: message,
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
