package token

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/domain"
)

const testSecret = "test-secret"

func TestManager_IssueVerify_RoundTrip(t *testing.T) {
	m := NewManager(testSecret)
	id := domain.SessionID(uuid.New())

	tok, err := m.Issue(id)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	got, err := m.Verify(tok)
	require.NoError(t, err)
	require.Equal(t, id, got)
}

func TestManager_Verify_Rejects(t *testing.T) {
	m := NewManager(testSecret)

	t.Run("empty token", func(t *testing.T) {
		_, err := m.Verify("")
		require.ErrorIs(t, err, errInvalidToken)
	})

	t.Run("garbage token", func(t *testing.T) {
		_, err := m.Verify("not.a.jwt")
		require.ErrorIs(t, err, errInvalidToken)
	})

	t.Run("signed with another secret", func(t *testing.T) {
		tok, err := NewManager("another-secret").Issue(domain.SessionID(uuid.New()))
		require.NoError(t, err)

		_, err = m.Verify(tok)
		require.ErrorIs(t, err, errInvalidToken)
	})

	t.Run("alg none is rejected", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
			Subject: uuid.NewString(),
		}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = m.Verify(tok)
		require.ErrorIs(t, err, errInvalidToken)
	})

	t.Run("subject is not a uuid", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Subject: "not-a-uuid",
		}).SignedString([]byte(testSecret))
		require.NoError(t, err)

		_, err = m.Verify(tok)
		require.ErrorIs(t, err, errInvalidToken)
	})
}
