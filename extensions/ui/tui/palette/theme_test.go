package palette

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTheme_ReturnsNonNil(t *testing.T) {
	theme := DefaultTheme()
	assert.NotNil(t, theme)
}

func TestDefaultTheme_HasExpectedColors(t *testing.T) {
	theme := DefaultTheme()

	// Base grayscale
	assert.Equal(t, "250", theme.Foreground)
	assert.Equal(t, "245", theme.ForegroundDim)
	assert.Equal(t, "15", theme.ForegroundBright)
	assert.Equal(t, "240", theme.Muted)
	assert.Equal(t, "248", theme.MutedBright)
	assert.Equal(t, "16", theme.Background)
	assert.Equal(t, "234", theme.BackgroundTint)
	assert.Equal(t, "236", theme.BackgroundTint2)

	// Structural
	assert.Equal(t, "240", theme.Border)
	assert.Equal(t, "248", theme.BorderFocused)
	assert.Equal(t, "114", theme.Success)
	assert.Equal(t, "167", theme.Error)
	assert.Equal(t, "172", theme.Warning)

	// State backgrounds
	assert.Equal(t, "235", theme.BackgroundTintPending)
	assert.Equal(t, "22", theme.BackgroundTintSuccess)
	assert.Equal(t, "52", theme.BackgroundTintError)

	// Dynamic accent (idle defaults)
	assert.Equal(t, "245", theme.Accent)
	assert.Equal(t, "243", theme.AccentDim)
	assert.Equal(t, "248", theme.AccentBright)
}

func TestDefaultTheme_ReturnsIndependentInstances(t *testing.T) {
	t1 := DefaultTheme()
	t2 := DefaultTheme()

	t1.Accent = "99"
	assert.Equal(t, "245", t2.Accent)
}
