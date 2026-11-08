package token

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"telegram-auth-extractor/internal/domain"
)

var errInvalidToken = errors.New("invalid session token")

const signingMethod = "HS256"

type Manager struct {
	secret []byte
}

func NewManager(secret string) *Manager {
	return &Manager{secret: []byte(secret)}
}

func (m *Manager) Issue(id domain.SessionID) (string, error) {
	const fn = "infra.token.Manager.Issue"

	claims := jwt.RegisteredClaims{
		Subject: uuid.UUID(id).String(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	return signed, nil
}

func (m *Manager) Verify(raw string) (domain.SessionID, error) {
	const fn = "infra.token.Manager.Verify"

	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{signingMethod}))

	if err != nil {
		return domain.SessionID{}, fmt.Errorf("%s: %w", fn, errInvalidToken)
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.SessionID{}, fmt.Errorf("%s: %w", fn, errInvalidToken)
	}

	return domain.SessionID(id), nil
}
