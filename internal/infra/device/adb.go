package device

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ADB struct {
	serial string
	binary string
}

func NewADB(serial, binary string) *ADB {
	if binary == "" {
		binary = "adb"
	}

	return &ADB{binary: binary, serial: serial}
}

func (a *ADB) Connect(ctx context.Context) error {
	out, err := a.runGlobal(ctx, "connect", a.serial)
	if err != nil {
		return fmt.Errorf("adb connect %s: %w", a.serial, err)
	}

	if !strings.Contains(out, "connected") {
		return fmt.Errorf("adb connect %s: %s", a.serial, out)
	}

	return nil
}

func (a *ADB) Root(ctx context.Context) error {
	out, err := a.runGlobal(ctx, "-s", a.serial, "root")
	if err != nil {
		return fmt.Errorf("adb root: %w", err)
	}

	if strings.Contains(out, "cannot run as root") {
		return fmt.Errorf("adb root: %s", out)
	}

	return nil
}

func (a *ADB) Pull(ctx context.Context, remote, local string) error {
	if _, err := a.run(ctx, "pull", remote, local); err != nil {
		return fmt.Errorf("adb pull %s: %w", remote, err)
	}

	return nil
}

func (a *ADB) Tap(ctx context.Context, x, y int) error {
	if _, err := a.Shell(ctx, "input", "tap", strconv.Itoa(x), strconv.Itoa(y)); err != nil {
		return fmt.Errorf("adb tap (%d,%d): %w", x, y, err)
	}

	return nil
}

func (a *ADB) InputText(ctx context.Context, text string) error {
	if _, err := a.Shell(ctx, "input", "text", escapeInputText(text)); err != nil {
		return fmt.Errorf("adb input text: %w", err)
	}

	return nil
}

func (a *ADB) KeyEvent(ctx context.Context, code int) error {
	if _, err := a.Shell(ctx, "input", "keyevent", strconv.Itoa(code)); err != nil {
		return fmt.Errorf("adb keyevent %d: %w", code, err)
	}

	return nil
}

func (a *ADB) ForceStop(ctx context.Context, pkg string) error {
	if _, err := a.Shell(ctx, "am", "force-stop", pkg); err != nil {
		return fmt.Errorf("adb force-stop %s: %w", pkg, err)
	}

	return nil
}

func (a *ADB) StartApp(ctx context.Context, pkg string) error {
	if _, err := a.Shell(ctx, "monkey", "-p", pkg,
		"-c", "android.intent.category.LAUNCHER",
		"--pct-syskeys", "0",
		"1"); err != nil {
		return fmt.Errorf("adb start %s: %w", pkg, err)
	}

	return nil
}

func (a *ADB) ClearApp(ctx context.Context, pkg string) error {
	out, err := a.Shell(ctx, "pm", "clear", pkg)
	if err != nil {
		return fmt.Errorf("adb pm clear %s: %w", pkg, err)
	}

	if !strings.Contains(out, "Success") {
		return fmt.Errorf("adb pm clear %s: %s", pkg, out)
	}

	return nil
}

func (a *ADB) Install(ctx context.Context, apkPath string) error {
	out, err := a.run(ctx, "install", "-r", apkPath)
	if err != nil {
		return fmt.Errorf("adb install: %w", err)
	}

	if !strings.Contains(out, "Success") {
		return fmt.Errorf("adb install: %s", out)
	}

	return nil
}

func (a *ADB) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	out, err := a.Shell(ctx, "pm", "list", "packages", pkg)
	if err != nil {
		return false, fmt.Errorf("adb pm list packages: %w", err)
	}

	return strings.Contains(out, "package:"+pkg), nil
}

func (a *ADB) Shell(ctx context.Context, args ...string) (string, error) {
	return a.run(ctx, append([]string{"shell"}, args...)...)
}

func (a *ADB) ShellRaw(ctx context.Context, args ...string) ([]byte, error) {
	return a.runRaw(ctx, append([]string{"exec-out"}, args...)...)
}

func (a *ADB) run(ctx context.Context, args ...string) (string, error) {
	out, err := a.runRaw(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func (a *ADB) runRaw(ctx context.Context, args ...string) ([]byte, error) {
	full := args
	if a.serial != "" {
		full = append([]string{"-s", a.serial}, args...)
	}

	return a.exec(ctx, full...)
}

func (a *ADB) exec(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return nil, fmt.Errorf("adb: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func (a *ADB) runGlobal(ctx context.Context, args ...string) (string, error) {
	out, err := a.exec(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func escapeInputText(text string) string {
	return strings.ReplaceAll(text, " ", "%s")
}
