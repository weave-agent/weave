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

	assert.Equal(t, "63", theme.Primary)
	assert.Equal(t, "60", theme.PrimaryDim)
	assert.Equal(t, "69", theme.PrimaryBright)
	assert.Equal(t, "84", theme.Success)
	assert.Equal(t, "204", theme.Error)
	assert.Equal(t, "221", theme.Warning)
	assert.Equal(t, "245", theme.Muted)
	assert.Equal(t, "252", theme.MutedBright)
	assert.Equal(t, "240", theme.Border)
	assert.Equal(t, "63", theme.BorderFocused)
	assert.Equal(t, "234", theme.BackgroundTint)
	assert.Equal(t, "15", theme.Foreground)
	assert.Equal(t, "15", theme.ForegroundBright)
}

func TestDefaultTheme_ReturnsIndependentInstances(t *testing.T) {
	t1 := DefaultTheme()
	t2 := DefaultTheme()

	// Modify t1 and verify t2 is unaffected
	t1.Primary = "99"
	assert.Equal(t, "63", t2.Primary)
}
