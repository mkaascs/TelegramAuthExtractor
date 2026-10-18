package device

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDownloadAPK(t *testing.T) {
	log := testLogger()

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("fake apk content"))
		}))
		defer srv.Close()

		path, err := downloadAPK(context.Background(), srv.URL, log)
		require.NoError(t, err)
		defer func(name string) {
			_ = os.Remove(name)
		}(path)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "fake apk content", string(data))
	})

	t.Run("empty url", func(t *testing.T) {
		_, err := downloadAPK(context.Background(), "", log)
		require.Error(t, err)
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := downloadAPK(context.Background(), srv.URL, log)
		require.Error(t, err)
	})

	t.Run("retries on broken connection then succeeds", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := calls.Add(1)
			if n < maxRetries {
				hj, ok := w.(http.Hijacker)
				require.True(t, ok, "ResponseWriter must support hijacking")
				conn, _, err := hj.Hijack()
				require.NoError(t, err)
				_ = conn.Close()
				return
			}

			_, _ = w.Write([]byte("apk after retries"))
		}))

		defer srv.Close()

		path, err := downloadAPK(context.Background(), srv.URL, log)
		require.NoError(t, err)
		defer func(name string) {
			_ = os.Remove(name)
		}(path)

		require.Equal(t, int32(maxRetries), calls.Load(), "должно быть ровно maxRetries попыток")

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "apk after retries", string(data))
	})

	t.Run("fails after all retries", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}))
		defer srv.Close()

		_, err := downloadAPK(context.Background(), srv.URL, log)
		require.Error(t, err)
		require.Equal(t, int32(maxRetries), calls.Load(), "должно исчерпать все попытки")
	})
}
