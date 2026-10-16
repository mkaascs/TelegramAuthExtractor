package device

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func loadDump(t *testing.T, name string) []Element {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "failed to read dump %s", name)

	elements, err := parseUIXML(data)
	require.NoError(t, err)
	require.NotEmpty(t, elements, "empty list")
	return elements
}

func TestParseUIXML_Smoke(t *testing.T) {
	elements := loadDump(t, "2_enter_phone.xml")

	require.Greater(t, len(elements), 10)

	var hasBounds bool
	for _, e := range elements {
		if e.Bounds != (Bounds{}) {
			hasBounds = true
			break
		}
	}

	require.True(t, hasBounds, "no one element was not parsed to bounds")
}

func TestParseUIXML_Invalid(t *testing.T) {
	_, err := parseUIXML([]byte("not xml at all <<<"))
	require.Error(t, err)
}

func TestMatchers_StartScreen(t *testing.T) {
	elements := loadDump(t, "1_start_messaging.xml")

	el, ok := Find(elements, startScreen())
	require.True(t, ok, "matcher startScreen didnt find 'Start Messaging'")
	require.Equal(t, "Start Messaging", el.Text)
	require.NotEqual(t, Bounds{}, el.Bounds, "the button must have bounds for tap")

	_, ok = Find(elements, phoneScreen())
	require.False(t, ok, "there is no screen with number entering on start screen")
}

func TestMatchers_PhoneScreen(t *testing.T) {
	elements := loadDump(t, "2_enter_phone.xml")

	t.Run("phone screen detected", func(t *testing.T) {
		_, ok := Find(elements, phoneScreen())
		require.True(t, ok, "matcher phoneScreen didnt find 'Your phone number'")
	})

	t.Run("country code field found with bounds", func(t *testing.T) {
		el, ok := Find(elements, countryCodeField())
		require.True(t, ok, "'Country code' was not found")
		require.NotEqual(t, Bounds{}, el.Bounds, "the field must have bounds for tap")
	})

	t.Run("done button found with bounds", func(t *testing.T) {
		el, ok := Find(elements, doneButton())
		require.True(t, ok, "the button 'Done' was not found")
		require.NotEqual(t, Bounds{}, el.Bounds)
	})

	t.Run("not the start screen anymore", func(t *testing.T) {
		_, ok := Find(elements, startScreen())
		require.False(t, ok, "there is no 'Start Messaging' on the screen for number")
	})

	t.Run("not the success screen", func(t *testing.T) {
		_, ok := Find(elements, successScreen())
		require.False(t, ok, "there are chats on the screen number")
	})
}

func TestMatchers_ScreensAreDistinct(t *testing.T) {
	start := loadDump(t, "1_start_messaging.xml")
	phone := loadDump(t, "2_enter_phone.xml")

	_, onStart := Find(start, startScreen())
	_, onPhone := Find(phone, startScreen())
	require.True(t, onStart)
	require.False(t, onPhone)

	_, onStart2 := Find(start, phoneScreen())
	_, onPhone2 := Find(phone, phoneScreen())
	require.False(t, onStart2)
	require.True(t, onPhone2)
}
