package sessions

import (
	"context"
	"github.com/google/uuid"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/domain"
)

type SessionInterface interface {
	CreateSession(context context.Context, phone string) (*domain.Session, bool, error)
	DeleteSession(context context.Context, sessionID uuid.UUID) error
	GetSession(context context.Context, sessionID uuid.UUID) (*domain.Session, error)
	SubmitCode(context context.Context, sessionID uuid.UUID, code string) (*domain.Session, error)
	GetSessionResult(context context.Context, sessionID uuid.UUID) (*domain.ExtractedData, error)
}

type SessionHandler struct {
	service SessionInterface
}

func NewSessionHandler(service SessionInterface) *SessionHandler {
	return &SessionHandler{service: service}
}

func (sh SessionHandler) CreateSession(ctx context.Context, request apiv1.CreateSessionRequestObject) (apiv1.CreateSessionResponseObject, error) {
	session, created, err := sh.service.CreateSession(ctx, request.Body.Phone)
	if err != nil {
		return ToCreateErrorResponse(err), nil
	}

	if !created {
		return apiv1.CreateSession200JSONResponse(ToDTOsSession(session)), nil
	}

	return apiv1.CreateSession202JSONResponse(ToDTOsSession(session)), nil
}

func (sh SessionHandler) DeleteSession(ctx context.Context, request apiv1.DeleteSessionRequestObject) (apiv1.DeleteSessionResponseObject, error) {
	err := sh.service.DeleteSession(ctx, request.Id)
	if err != nil {
		return ToDeleteErrorResponse(err), nil
	}

	return apiv1.DeleteSession204Response{}, nil
}

func (sh SessionHandler) GetSession(ctx context.Context, request apiv1.GetSessionRequestObject) (apiv1.GetSessionResponseObject, error) {
	session, err := sh.service.GetSession(ctx, request.Id)
	if err != nil {
		return ToGetErrorResponse(err), nil
	}

	return apiv1.GetSession200JSONResponse(ToDTOsSession(session)), nil
}

func (sh SessionHandler) SubmitCode(ctx context.Context, request apiv1.SubmitCodeRequestObject) (apiv1.SubmitCodeResponseObject, error) {
	session, err := sh.service.SubmitCode(ctx, request.Id, request.Body.Code)
	if err != nil {
		return ToSubmitErrorResponse(err), nil
	}

	return apiv1.SubmitCode202JSONResponse(ToDTOsSession(session)), nil
}

func (sh SessionHandler) GetSessionResult(ctx context.Context, request apiv1.GetSessionResultRequestObject) (apiv1.GetSessionResultResponseObject, error) {
	result, err := sh.service.GetSessionResult(ctx, request.Id)
	if err != nil {
		return ToGetResultErrorResponse(err), nil
	}

	return apiv1.GetSessionResult200JSONResponse(ToDTOsExtractedData(result)), nil
}
