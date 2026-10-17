package device

import (
	"context"
	"fmt"
	"log/slog"

	"telegram-auth-extractor/internal/config"
)

type Device struct {
	adb *ADB
	cfg config.ADB

	sessionFile string
}

func NewDevice(cfg config.ADB, log *slog.Logger) *Device {
	return &Device{
		adb:         NewADB(cfg.Serial, cfg.Binary),
		cfg:         cfg,
		sessionFile: fmt.Sprintf("/data/data/%s/files/tgnet.dat", cfg.Package),
	}
}

func (d *Device) Prepare(ctx context.Context) error {
	const fn = "adapters.device.Device.Prepare"

	if err := d.adb.Connect(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := d.adb.Root(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Device) PrepareTelegram(ctx context.Context) error {
	const fn = "adapters.device.Device.PrepareTelegram"

	if err := d.adb.ForceStop(ctx, d.cfg.Package); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := d.adb.StartApp(ctx, d.cfg.Package); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := d.ensureStarted(ctx); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Device) EnterPhone(ctx context.Context, phone string) error {
	const fn = "adapters.device.Device.EnterPhone"

	if err := d.submitPhone(ctx, phone); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Device) EnterCode(ctx context.Context, code string) error {
	const fn = "adapters.device.Device.EnterCode"

	if err := d.submitCode(ctx, code); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Device) SessionFile(ctx context.Context) ([]byte, error) {
	const fn = "adapters.device.Device.SessionFile"

	data, err := d.adb.ShellRaw(ctx, "su", "0", "cat", d.sessionFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%s: empty session file (not logged in?)", fn)
	}

	return data, nil
}

func (d *Device) Clear(ctx context.Context) error {
	const fn = "adapters.device.Device.Clear"

	if err := d.adb.ClearApp(ctx, d.cfg.Package); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}
