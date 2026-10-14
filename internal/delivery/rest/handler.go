package rest

import (
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/delivery/rest/sessions"
)

type Service interface {
	sessions.SessionInterface
}

type Handler struct {
	*sessions.SessionHandler
}

func NewHandler(service Service) apiv1.ServerInterface {
	return apiv1.NewStrictHandler(Handler{
		SessionHandler: sessions.NewSessionHandler(service),
	}, nil)
}
