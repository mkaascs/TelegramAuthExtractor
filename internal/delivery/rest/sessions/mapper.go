package sessions

import (
	"errors"
	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/domain"

	"github.com/oapi-codegen/runtime/types"
)

func ToDTOsSession(session *domain.Session) apiv1.Session {
	return apiv1.Session{
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
		Error:        session.Error,
		Id:           types.UUID(session.ID),
		Phone:        session.Phone,
		Status:       apiv1.SessionStatus(session.Status),
		CodeAttempts: session.CodeAttempts,
	}
}

func ToDTOsSessionWithToken(session *domain.Session, token string) apiv1.SessionWithToken {
	return apiv1.SessionWithToken{
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
		Error:        session.Error,
		Id:           types.UUID(session.ID),
		Phone:        session.Phone,
		Status:       apiv1.SessionStatus(session.Status),
		CodeAttempts: session.CodeAttempts,
		SessionToken: token,
	}
}

func ToDTOsSessionCreated(session *domain.Session, token, decryptionKey string) apiv1.SessionCreated {
	return apiv1.SessionCreated{
		CreatedAt:     session.CreatedAt,
		ExpiresAt:     session.ExpiresAt,
		Error:         session.Error,
		Id:            types.UUID(session.ID),
		Phone:         session.Phone,
		Status:        apiv1.SessionStatus(session.Status),
		CodeAttempts:  session.CodeAttempts,
		SessionToken:  token,
		DecryptionKey: decryptionKey,
	}
}

func ToDTOsExtractedData(data *domain.EncryptedResult) apiv1.ExtractedData {
	return apiv1.ExtractedData{
		AuthKey: data.AuthKeyEncrypted,
		DcId:    data.DCID,
		UserId:  data.UserID,
	}
}

func ToCreateErrorResponse(_ error) apiv1.CreateSessionResponseObject {
	return apiv1.CreateSession500JSONResponse(
		apiv1.Error{Code: new("internal_error"), Message: "internal server error"})
}

func ToDeleteErrorResponse(err error) apiv1.DeleteSessionResponseObject {
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return apiv1.DeleteSession404JSONResponse(
			apiv1.Error{Code: new("session_not_found"), Message: "session not found"})
	default:
		return apiv1.DeleteSession500JSONResponse(
			apiv1.Error{Code: new("internal_error"), Message: "internal server error"})
	}
}

func ToGetErrorResponse(err error) apiv1.GetSessionResponseObject {
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return apiv1.GetSession404JSONResponse(
			apiv1.Error{Code: new("session_not_found"), Message: "session not found"})
	default:
		return apiv1.GetSession500JSONResponse(
			apiv1.Error{Code: new("internal_error"), Message: "internal server error"})
	}
}

func ToSubmitErrorResponse(err error) apiv1.SubmitCodeResponseObject {
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return apiv1.SubmitCode404JSONResponse(
			apiv1.Error{Code: new("session_not_found"), Message: "session not found"})
	case errors.Is(err, domain.ErrWrongState):
		return apiv1.SubmitCode409JSONResponse(
			apiv1.Error{Code: new("session_not_waiting_for_code"), Message: "current session is not waiting for code"})
	default:
		return apiv1.SubmitCode500JSONResponse(
			apiv1.Error{Code: new("internal_error"), Message: "internal server error"})
	}
}

func ToGetResultErrorResponse(err error) apiv1.GetSessionResultResponseObject {
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return apiv1.GetSessionResult404JSONResponse(
			apiv1.Error{Code: new("session_not_found"), Message: "session not found"})
	case errors.Is(err, domain.ErrWrongState):
		return apiv1.GetSessionResult409JSONResponse(
			apiv1.Error{Code: new("session_not_done"), Message: "current session is not done"})
	default:
		return apiv1.GetSessionResult500JSONResponse(
			apiv1.Error{Code: new("internal_error"), Message: "internal server error"})
	}
}