package device

import (
	"context"
	"fmt"
	"telegram-auth-extractor/internal/domain"
	"time"
)

const (
	keyMoveEnd = 123 // KEYCODE_MOVE_END
	keyDel     = 67  // KEYCODE_DEL
)

func startScreen() Matcher {
	return ByText("Start Messaging")
}

func phoneScreen() Matcher {
	return ByText("Your phone number")
}

func phoneField() Matcher { return ByContentDesc("Phone number") }

func codeScreen() Matcher {
	return ByTextContains("Check your Telegram")
}

func codeErrorScreen() Matcher {
	return ByTextContains("wrong code")
}

func successScreen() Matcher {
	return Or(ByText("Search Chats"), ByTextContains("unread chats"))
}

func countryCodeField() Matcher {
	return ByContentDesc("Country code")
}

func doneButton() Matcher {
	return ByContentDesc("Done")
}

func codeField() Matcher {
	return byClass("android.widget.EditText")
}

func byClass(c string) Matcher {
	return func(e Element) bool { return e.Class == c }
}

var dismissables = []struct {
	name    string
	trigger Matcher
	close   Matcher
}{
	{"update dialog", ByTextContains("Update Telegram"), ByText("Remind me later")},
	{"call log request", ByTextContains("read the call log"), ByText("Continue")},
	{"lock screen request", ByTextContains("shown on the lock screen"), ByText("Not now")},
	{"receive calls request", ByTextContains("receive calls"), ByText("Continue")},
	{"permission dialog",
		ByResourceID("com.android.permissioncontroller:id/permission_allow_button"),
		ByResourceID("com.android.permissioncontroller:id/permission_allow_button")},
	{"anr dialog",
		ByTextContains("isn't responding"),
		ByResourceID("android:id/aerr_wait")},
	{"confirm number",
		ByTextContains("correct number"),
		ByText("Yes")},
}

func (d *Device) dismissFound(ctx context.Context, elements []Element) {
	for _, dismissable := range dismissables {
		if _, ok := Find(elements, dismissable.trigger); !ok {
			continue
		}

		if btn, ok := Find(elements, dismissable.close); ok {
			x, y := btn.Center()
			_ = d.adb.Tap(ctx, x, y)
		}
	}
}

func (d *Device) waitScreenTimeout(ctx context.Context, matchers map[string]Matcher, timeout time.Duration) (string, Element, error) {
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(d.cfg.WaitInterval)
	defer ticker.Stop()

	for {
		if elements, err := dumpUI(ctx, d.adb); err == nil {
			for key, matcher := range matchers {
				if el, ok := Find(elements, matcher); ok {
					return key, el, nil
				}
			}

			d.dismissFound(ctx, elements)
		}

		if time.Now().After(deadline) {
			return "", Element{}, fmt.Errorf("waitScreen: %w", domain.ErrScreenTimeout)
		}

		select {
		case <-ctx.Done():
			return "", Element{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (d *Device) waitOneTimeout(ctx context.Context, match Matcher, timeout time.Duration) (Element, error) {
	_, el, err := d.waitScreenTimeout(ctx, map[string]Matcher{"": match}, timeout)
	return el, err
}

func (d *Device) find(ctx context.Context, match Matcher) (Element, bool, error) {
	elements, err := dumpUI(ctx, d.adb)
	if err != nil {
		return Element{}, false, err
	}

	el, ok := Find(elements, match)
	return el, ok, nil
}

func (d *Device) tapElement(ctx context.Context, el Element) error {
	x, y := el.Center()
	return d.adb.Tap(ctx, x, y)
}

func (d *Device) clearField(ctx context.Context) error {
	if err := d.adb.KeyEvent(ctx, keyMoveEnd); err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		if err := d.adb.KeyEvent(ctx, keyDel); err != nil {
			return err
		}
	}

	return nil
}

func (d *Device) ensureStarted(ctx context.Context) error {
	const fn = "infra.device.Device.ensureStarted"

	key, el, err := d.waitScreenTimeout(ctx, map[string]Matcher{
		"start": startScreen(),
		"phone": phoneScreen(),
	}, d.cfg.WaitTimeout)

	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if key == "start" {
		if err := d.tapElement(ctx, el); err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}

		if _, err := d.waitOneTimeout(ctx, phoneScreen(), d.cfg.WaitTimeout); err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}
	}

	return nil
}

func (d *Device) submitPhone(ctx context.Context, phone string) error {
	const fn = "infra.device.Device.submitPhone"

	if _, err := d.waitOneTimeout(ctx, phoneScreen(), d.cfg.PhoneCheckTimeout); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if numField, ok, err := d.find(ctx, phoneField()); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	} else if ok {
		if err := d.tapElement(ctx, numField); err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}

		_ = d.clearField(ctx)
	}

	ccField, ok, err := d.find(ctx, countryCodeField())
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if !ok {
		return fmt.Errorf("%s: country code field not found", fn)
	}

	if err := d.tapElement(ctx, ccField); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	_ = d.clearField(ctx)
	if err := d.adb.InputText(ctx, phone); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	btn, ok, err := d.find(ctx, doneButton())
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if !ok {
		return fmt.Errorf("%s: done button not found", fn)
	}

	if err := d.tapElement(ctx, btn); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if _, err := d.waitOneTimeout(ctx, codeScreen(), d.cfg.WaitTimeout); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	return nil
}

func (d *Device) submitCode(ctx context.Context, code string) error {
	const fn = "infra.device.Device.submitCode"

	field, ok, err := d.find(ctx, codeField())
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if !ok {
		return fmt.Errorf("%s: code field not found", fn)
	}

	if err := d.tapElement(ctx, field); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if err := d.adb.InputText(ctx, code); err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	key, _, err := d.waitScreenTimeout(ctx, map[string]Matcher{
		"success": successScreen(),
		"error":   codeErrorScreen(),
	}, d.cfg.WaitTimeout)

	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if key == "error" {
		return fmt.Errorf("%s: %w", fn, domain.ErrCodeInvalid)
	}

	return nil
}
