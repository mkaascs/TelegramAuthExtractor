package sessions

import (
	"context"
	"errors"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/delivery/rest/middlewares"
	"telegram-auth-extractor/internal/domain"

	"github.com/google/uuid"
)

var errNoSessionID = errors.New("session id missing from request context")

type SessionInterface interface {
	CreateSession(context context.Context, phone string) (*domain.CreateSessionResult, error)
	DeleteSession(context context.Context, sessionID uuid.UUID) error
	GetSession(context context.Context, sessionID uuid.UUID) (*domain.Session, error)
	SubmitCode(context context.Context, sessionID uuid.UUID, code string) error
	GetSessionResult(context context.Context, sessionID uuid.UUID) (*domain.EncryptedResult, error)
}

type SessionHandler struct {
	service SessionInterface
}

func NewSessionHandler(service SessionInterface) *SessionHandler {
	return &SessionHandler{service: service}
}

func (sh SessionHandler) CreateSession(ctx context.Context, request apiv1.CreateSessionRequestObject) (apiv1.CreateSessionResponseObject, error) {
	result, err := sh.service.CreateSession(ctx, request.Body.Phone)
	if err != nil {
		return ToCreateErrorResponse(err), nil
	}

	if !result.Created {
		return apiv1.CreateSession200JSONResponse(ToDTOsSessionWithToken(result.Session, result.Token)), nil
	}

	return apiv1.CreateSession202JSONResponse(ToDTOsSessionCreated(result.Session, result.Token, result.DecryptionKey)), nil
}

func (sh SessionHandler) DeleteSession(ctx context.Context, _ apiv1.DeleteSessionRequestObject) (apiv1.DeleteSessionResponseObject, error) {
	id, ok := middlewares.GetSessionID(ctx)
	if !ok {
		return ToDeleteErrorResponse(errNoSessionID), nil
	}

	if err := sh.service.DeleteSession(ctx, uuid.UUID(id)); err != nil {
		return ToDeleteErrorResponse(err), nil
	}

	return apiv1.DeleteSession204Response{}, nil
}

func (sh SessionHandler) GetSession(ctx context.Context, _ apiv1.GetSessionRequestObject) (apiv1.GetSessionResponseObject, error) {
	id, ok := middlewares.GetSessionID(ctx)
	if !ok {
		return ToGetErrorResponse(errNoSessionID), nil
	}

	session, err := sh.service.GetSession(ctx, uuid.UUID(id))
	if err != nil {
		return ToGetErrorResponse(err), nil
	}

	return apiv1.GetSession200JSONResponse(ToDTOsSession(session)), nil
}

func (sh SessionHandler) SubmitCode(ctx context.Context, request apiv1.SubmitCodeRequestObject) (apiv1.SubmitCodeResponseObject, error) {
	id, ok := middlewares.GetSessionID(ctx)
	if !ok {
		return ToSubmitErrorResponse(errNoSessionID), nil
	}

	if err := sh.service.SubmitCode(ctx, uuid.UUID(id), request.Body.Code); err != nil {
		return ToSubmitErrorResponse(err), nil
	}

	return apiv1.SubmitCode202Response{}, nil
}

func (sh SessionHandler) GetSessionResult(ctx context.Context, _ apiv1.GetSessionResultRequestObject) (apiv1.GetSessionResultResponseObject, error) {
	id, ok := middlewares.GetSessionID(ctx)
	if !ok {
		return ToGetResultErrorResponse(errNoSessionID), nil
	}

	result, err := sh.service.GetSessionResult(ctx, uuid.UUID(id))
	if err != nil {
		return ToGetResultErrorResponse(err), nil
	}

	return apiv1.GetSessionResult200JSONResponse(ToDTOsExtractedData(result)), nil
}
