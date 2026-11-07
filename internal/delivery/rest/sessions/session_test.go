package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apiv1 "telegram-auth-extractor/gen/rest/v1"
	"telegram-auth-extractor/internal/delivery/rest/middlewares"
	"telegram-auth-extractor/internal/domain"
)

type mockService struct {
	createFn    func(ctx context.Context, phone string) (*domain.CreateSessionResult, error)
	deleteFn    func(ctx context.Context, id uuid.UUID) error
	getFn       func(ctx context.Context, id uuid.UUID) (*domain.Session, error)
	submitFn    func(ctx context.Context, id uuid.UUID, code string) error
	getResultFn func(ctx context.Context, id uuid.UUID) (*domain.EncryptedResult, error)
}

func (m mockService) CreateSession(ctx context.Context, phone string) (*domain.CreateSessionResult, error) {
	return m.createFn(ctx, phone)
}

func (m mockService) DeleteSession(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

func (m mockService) GetSession(ctx context.Context, id uuid.UUID) (*domain.Session, error) {
	return m.getFn(ctx, id)
}

func (m mockService) SubmitCode(ctx context.Context, id uuid.UUID, code string) error {
	return m.submitFn(ctx, id, code)
}

func (m mockService) GetSessionResult(ctx context.Context, id uuid.UUID) (*domain.EncryptedResult, error) {
	return m.getResultFn(ctx, id)
}

func fakeSession() *domain.Session {
	return &domain.Session{
		ID:        domain.SessionID(uuid.New()),
		Phone:     "+79991234567",
		Status:    "waiting_for_code",
		CreatedAt: time.Now(),
	}
}

func TestSessionHandler_CreateSession(t *testing.T) {
	ctx := context.Background()
	const phone = "+79991234567"

	t.Run("created -> 202", func(t *testing.T) {
		sess := fakeSession()
		h := NewSessionHandler(mockService{
			createFn: func(_ context.Context, p string) (*domain.CreateSessionResult, error) {
				require.Equal(t, phone, p)
				return &domain.CreateSessionResult{
					Session:       sess,
					Token:         "test-token",
					DecryptionKey: "test-decryption-key",
					Created:       true,
				}, nil
			},
		})

		resp, err := h.CreateSession(ctx, apiv1.CreateSessionRequestObject{
			Body: &apiv1.CreateSessionRequest{Phone: phone},
		})
		require.NoError(t, err)

		got, ok := resp.(apiv1.CreateSession202JSONResponse)
		require.True(t, ok, "expected 202, got %T", resp)
		require.Equal(t, sess.Phone, got.Phone)
		require.Equal(t, "test-decryption-key", got.DecryptionKey)
	})

	t.Run("idempotent (created=false) -> 200", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			createFn: func(_ context.Context, _ string) (*domain.CreateSessionResult, error) {
				return &domain.CreateSessionResult{
					Session: fakeSession(),
					Token:   "test-token",
					Created: false,
				}, nil
			},
		})

		resp, err := h.CreateSession(ctx, apiv1.CreateSessionRequestObject{
			Body: &apiv1.CreateSessionRequest{Phone: phone},
		})
		require.NoError(t, err)

		_, ok := resp.(apiv1.CreateSession200JSONResponse)
		require.True(t, ok, "expected 200, got %T", resp)
	})

	t.Run("service error -> 500", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			createFn: func(_ context.Context, _ string) (*domain.CreateSessionResult, error) {
				return nil, errors.New("boom")
			},
		})

		resp, err := h.CreateSession(ctx, apiv1.CreateSessionRequestObject{
			Body: &apiv1.CreateSessionRequest{Phone: phone},
		})

		require.NoError(t, err)

		got, ok := resp.(apiv1.CreateSession500JSONResponse)
		require.True(t, ok, "expected 500, got %T", resp)
		require.Equal(t, "internal_error", *got.Code)
	})
}

func TestSessionHandler_GetSession(t *testing.T) {
	id := uuid.New()
	ctx := middlewares.WithSessionID(context.Background(), domain.SessionID(id))

	t.Run("found -> 200", func(t *testing.T) {
		sess := fakeSession()
		sess.ID = domain.SessionID(id)
		h := NewSessionHandler(mockService{
			getFn: func(_ context.Context, got uuid.UUID) (*domain.Session, error) {
				require.Equal(t, id, got)
				return sess, nil
			},
		})

		resp, err := h.GetSession(ctx, apiv1.GetSessionRequestObject{})
		require.NoError(t, err)

		_, ok := resp.(apiv1.GetSession200JSONResponse)
		require.True(t, ok, "expected 200, got %T", resp)
	})

	t.Run("not found -> 404", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			getFn: func(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
				return nil, domain.ErrSessionNotFound
			},
		})

		resp, err := h.GetSession(ctx, apiv1.GetSessionRequestObject{})
		require.NoError(t, err)

		got, ok := resp.(apiv1.GetSession404JSONResponse)
		require.True(t, ok, "expected 404, got %T", resp)
		require.Equal(t, "session_not_found", *got.Code)
	})
}

func TestSessionHandler_SubmitCode(t *testing.T) {
	id := uuid.New()
	ctx := middlewares.WithSessionID(context.Background(), domain.SessionID(id))
	const code = "12345"

	t.Run("accepted -> 202", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			submitFn: func(_ context.Context, _ uuid.UUID, c string) error {
				require.Equal(t, code, c)
				return nil
			},
		})

		resp, err := h.SubmitCode(ctx, apiv1.SubmitCodeRequestObject{
			Body: &apiv1.SubmitCodeRequest{Code: code},
		})
		require.NoError(t, err)

		_, ok := resp.(apiv1.SubmitCode202Response)
		require.True(t, ok, "expected 202, got %T", resp)
	})

	t.Run("wrong state -> 409", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			submitFn: func(_ context.Context, _ uuid.UUID, _ string) error {
				return domain.ErrWrongState
			},
		})

		resp, err := h.SubmitCode(ctx, apiv1.SubmitCodeRequestObject{
			Body: &apiv1.SubmitCodeRequest{Code: code},
		})
		require.NoError(t, err)

		got, ok := resp.(apiv1.SubmitCode409JSONResponse)
		require.True(t, ok, "expected 409, got %T", resp)
		require.Equal(t, "session_not_waiting_for_code", *got.Code)
	})

	t.Run("not found -> 404", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			submitFn: func(_ context.Context, _ uuid.UUID, _ string) error {
				return domain.ErrSessionNotFound
			},
		})

		resp, err := h.SubmitCode(ctx, apiv1.SubmitCodeRequestObject{
			Body: &apiv1.SubmitCodeRequest{Code: code},
		})
		require.NoError(t, err)

		_, ok := resp.(apiv1.SubmitCode404JSONResponse)
		require.True(t, ok, "expected 404, got %T", resp)
	})
}

func TestSessionHandler_GetSessionResult(t *testing.T) {
	id := uuid.New()
	ctx := middlewares.WithSessionID(context.Background(), domain.SessionID(id))

	t.Run("done -> 200 with data", func(t *testing.T) {
		data := &domain.EncryptedResult{
			DCID:             2,
			AuthKeyEncrypted: "-----BEGIN AGE ENCRYPTED FILE-----...",
			UserID:           1153367122,
		}
		h := NewSessionHandler(mockService{
			getResultFn: func(_ context.Context, _ uuid.UUID) (*domain.EncryptedResult, error) {
				return data, nil
			},
		})

		resp, err := h.GetSessionResult(ctx, apiv1.GetSessionResultRequestObject{})
		require.NoError(t, err)

		got, ok := resp.(apiv1.GetSessionResult200JSONResponse)
		require.True(t, ok, "expected 200, got %T", resp)
		require.Equal(t, 2, got.DcId)
		require.Equal(t, int64(1153367122), got.UserId)
		require.Equal(t, data.AuthKeyEncrypted, got.AuthKey)
	})

	t.Run("not ready -> 409", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			getResultFn: func(_ context.Context, _ uuid.UUID) (*domain.EncryptedResult, error) {
				return nil, domain.ErrWrongState
			},
		})

		resp, err := h.GetSessionResult(ctx, apiv1.GetSessionResultRequestObject{})
		require.NoError(t, err)

		got, ok := resp.(apiv1.GetSessionResult409JSONResponse)
		require.True(t, ok, "expected 409, got %T", resp)
		require.Equal(t, "session_not_done", *got.Code)
	})

	t.Run("not found -> 404", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			getResultFn: func(_ context.Context, _ uuid.UUID) (*domain.EncryptedResult, error) {
				return nil, domain.ErrSessionNotFound
			},
		})

		resp, err := h.GetSessionResult(ctx, apiv1.GetSessionResultRequestObject{})
		require.NoError(t, err)

		_, ok := resp.(apiv1.GetSessionResult404JSONResponse)
		require.True(t, ok, "expected 404, got %T", resp)
	})
}

func TestSessionHandler_DeleteSession(t *testing.T) {
	id := uuid.New()
	ctx := middlewares.WithSessionID(context.Background(), domain.SessionID(id))

	t.Run("deleted -> 204", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			deleteFn: func(_ context.Context, _ uuid.UUID) error { return nil },
		})

		resp, err := h.DeleteSession(ctx, apiv1.DeleteSessionRequestObject{})
		require.NoError(t, err)

		_, ok := resp.(apiv1.DeleteSession204Response)
		require.True(t, ok, "expected 204, got %T", resp)
	})

	t.Run("not found -> 404", func(t *testing.T) {
		h := NewSessionHandler(mockService{
			deleteFn: func(_ context.Context, _ uuid.UUID) error { return domain.ErrSessionNotFound },
		})

		resp, err := h.DeleteSession(ctx, apiv1.DeleteSessionRequestObject{})
		require.NoError(t, err)

		got, ok := resp.(apiv1.DeleteSession404JSONResponse)
		require.True(t, ok, "expected 404, got %T", resp)
		require.Equal(t, "session_not_found", *got.Code)
	})
}
