package device

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBounds(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		b, err := ParseBounds("[48,560][1032,680]")
		require.NoError(t, err)
		require.Equal(t, Bounds{X1: 48, Y1: 560, X2: 1032, Y2: 680}, b)
	})

	t.Run("zeros", func(t *testing.T) {
		b, err := ParseBounds("[0,0][0,0]")
		require.NoError(t, err)
		require.Equal(t, Bounds{}, b)
	})

	t.Run("embedded in larger string", func(t *testing.T) {
		b, err := ParseBounds("bounds=[10,20][30,40] class=...")
		require.NoError(t, err)
		require.Equal(t, Bounds{X1: 10, Y1: 20, X2: 30, Y2: 40}, b)
	})

	t.Run("bad format", func(t *testing.T) {
		for _, s := range []string{"", "[1,2]", "garbage", "[a,b][c,d]"} {
			_, err := ParseBounds(s)
			require.Error(t, err, "должна быть ошибка на %q", s)
		}
	})
}

func TestBounds_Center(t *testing.T) {
	cases := []struct {
		name   string
		b      Bounds
		wx, wy int
	}{
		{"simple", Bounds{0, 0, 100, 200}, 50, 100},
		{"offset", Bounds{48, 560, 1032, 680}, 540, 620},
		{"point", Bounds{10, 10, 10, 10}, 10, 10},
		{"odd rounds down", Bounds{0, 0, 3, 5}, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y := tc.b.Center()
			require.Equal(t, tc.wx, x)
			require.Equal(t, tc.wy, y)
		})
	}
}

func TestElement_Center(t *testing.T) {
	e := Element{Bounds: Bounds{X1: 0, Y1: 0, X2: 100, Y2: 40}}
	x, y := e.Center()
	require.Equal(t, 50, x)
	require.Equal(t, 20, y)
}

func TestByText(t *testing.T) {
	m := ByText("Enter code")
	require.True(t, m(Element{Text: "Enter code"}))
	require.False(t, m(Element{Text: "Enter the code"}))
	require.False(t, m(Element{Text: "enter code"}))
}

func TestByTextContains(t *testing.T) {
	m := ByTextContains("code")
	require.True(t, m(Element{Text: "Enter code"}))
	require.True(t, m(Element{Text: "ENTER CODE"}))
	require.True(t, m(Element{Text: "the code is"}))
	require.False(t, m(Element{Text: "password"}))
}

func TestByResourceID(t *testing.T) {
	m := ByResourceID("org.telegram.messenger.web:id/code")
	require.True(t, m(Element{ResourceID: "org.telegram.messenger.web:id/code"}))
	require.False(t, m(Element{ResourceID: "org.telegram.messenger.web:id/phone"}))
}

func TestByContentDesc(t *testing.T) {
	m := ByContentDesc("Next")
	require.True(t, m(Element{ContentDesc: "Next"}))
	require.False(t, m(Element{ContentDesc: "Back"}))
}

func TestAnd(t *testing.T) {
	m := And(ByResourceID("id/code"), ByTextContains("code"))

	require.True(t, m(Element{ResourceID: "id/code", Text: "Enter code"}))
	require.False(t, m(Element{ResourceID: "id/code", Text: "phone"}))
	require.False(t, m(Element{ResourceID: "id/phone", Text: "code"}))

	require.True(t, And()(Element{}))
}

func TestOr(t *testing.T) {
	m := Or(ByTextContains("code"), ByTextContains("password"))

	require.True(t, m(Element{Text: "Enter code"}))
	require.True(t, m(Element{Text: "Your password"}))
	require.False(t, m(Element{Text: "phone number"}))

	require.False(t, Or()(Element{}))
}

func TestFind(t *testing.T) {
	elements := []Element{
		{Text: "Enter your phone number", ResourceID: "id/phone"},
		{Text: "Next", ResourceID: "id/next", Clickable: true},
		{Text: "Enter code", ResourceID: "id/code"},
	}

	t.Run("found returns first match", func(t *testing.T) {
		e, ok := Find(elements, ByResourceID("id/code"))
		require.True(t, ok)
		require.Equal(t, "Enter code", e.Text)
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := Find(elements, ByResourceID("id/missing"))
		require.False(t, ok)
	})

	t.Run("first of several", func(t *testing.T) {
		e, ok := Find(elements, ByTextContains("enter"))
		require.True(t, ok)
		require.Equal(t, "id/phone", e.ResourceID)
	})

	t.Run("empty slice", func(t *testing.T) {
		_, ok := Find(nil, ByText("x"))
		require.False(t, ok)
	})
}
