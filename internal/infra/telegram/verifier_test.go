//go:build integration
// +build integration

package telegram

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"strconv"
	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifier_ReauthWithExtractedKey(t *testing.T) {
	authKeyB64 := os.Getenv("TELEGRAM_AUTH_KEY")
	apiID := os.Getenv("TELEGRAM_API_ID")
	apiHash := os.Getenv("TELEGRAM_API_HASH")
	if authKeyB64 == "" {
		t.Skip("TELEGRAM_AUTH_KEY not set — skipping re-auth integration test")
	}

	authKey, err := base64.StdEncoding.DecodeString(authKeyB64)
	require.NoError(t, err)
	require.Len(t, authKey, 256)

	dc := 5
	if raw := os.Getenv("TELEGRAM_DC"); raw != "" {
		dc = parseInt(raw)
	}

	verifier := NewVerifier(config.Telegram{ApiID: parseInt(apiID), ApiHash: apiHash}, discardLogger())

	result, err := verifier.Verify(context.Background(), []domain.DatacenterKey{
		{DCID: dc, AuthKey: authKey},
	})

	require.NoError(t, err, "re‑auth using the extracted key must be successful")
	require.NotZero(t, result.UserID, "user_id must be returned")

	t.Logf("✅ reauth ok: dc=%d user_id=%d", result.DCID, result.UserID)
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
