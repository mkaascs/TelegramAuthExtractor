package server

import (
	"context"
	"fmt"
	"net/http"
	"telegram-auth-extractor/internal/config"
)

type Server struct {
	http *http.Server
}

func NewServer(cfg config.Config, handler http.Handler) *Server {
	return &Server{http: &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Http.Port),
		Handler:      handler,
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
