package device

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"telegram-auth-extractor/internal/lib/logging/sloglib"
	"time"
)

const (
	maxRetries         = 3
	apkDownloadTimeout = 15 * time.Minute
)

func (d *Device) ensureTelegram(ctx context.Context) error {
	const fn = "infra.device.Device.ensureTelegram"
	log := d.log.With(slog.String("fn", fn), slog.String("serial", d.cfg.Serial))

	installed, err := d.adb.IsInstalled(ctx, d.cfg.Package)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if installed {
		log.Info("telegram already installed, skipping install", slog.String("package", d.cfg.Package))
		return nil
	}

	log.Info("telegram is not installed, downloading apk", slog.String("apk_url", d.cfg.ApkURL))

	apkPath, err := downloadAPK(ctx, d.cfg.ApkURL, log)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			log.Warn("failed to remove apk file", slog.String("name", name))
		}
	}(apkPath)

	if err := d.adb.Install(ctx, apkPath); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	log.Info("telegram installed", slog.String("package", d.cfg.Package))
	return nil
}

func downloadAPK(ctx context.Context, url string, log *slog.Logger) (string, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		path, err := tryDownloadAPK(ctx, url, log)
		if err == nil {
			return path, nil
		}

		lastErr = err
		log.Debug("apk download attempt failed, retrying", slog.Int("attempt", attempt+1), sloglib.Error(err))
	}

	return "", fmt.Errorf("download apk after retries: %w", lastErr)
}

func tryDownloadAPK(ctx context.Context, url string, log *slog.Logger) (string, error) {
	if url == "" {
		return "", fmt.Errorf("apk url is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, apkDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	log.Debug("requesting apk", slog.String("apk_url", url))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download apk: %w", err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Warn("failed to close apk body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download apk: unexpected status %d", resp.StatusCode)
	}

	log.Debug("apk response received",
		slog.Int("status", resp.StatusCode),
		slog.Int64("expected_bytes", resp.ContentLength))

	file, err := os.CreateTemp("", "telegram-*.apk")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Warn("failed to close temp file", slog.String("file", file.Name()))
		}
	}(file)

	written, err := io.Copy(file, resp.Body)
	if err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("save apk: %w", err)
	}

	log.Debug("apk downloaded", slog.Int64("bytes", written), slog.String("file", file.Name()))
	return file.Name(), nil
}
