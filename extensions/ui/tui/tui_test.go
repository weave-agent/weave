package tui

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUI_ExtensionRegistration(t *testing.T) {
	sdk.ResetRegistry()
	defer sdk.ResetRegistry()

	sdk.RegisterExtension("tui", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewTUI(cfg)
	})

	ext, err := sdk.GetExtension("tui", nil)
	require.NoError(t, err)
	assert.Equal(t, "tui", ext.Name())

	_, ok := ext.(*TUI)
	require.True(t, ok, "expected *TUI, got %T", ext)
}

func TestTUI_Name(t *testing.T) {
	tui, err := NewTUI(nil)
	require.NoError(t, err)
	assert.Equal(t, "tui", tui.Name())
}

func TestTUI_CloseWithoutSubscribe(t *testing.T) {
	tui, err := NewTUI(nil)
	require.NoError(t, err)

	// Close without Subscribe should not panic or block
	require.NoError(t, tui.Close())
}

func TestModel_View(t *testing.T) {
	m := newModel(nil, nil)
	// View now includes editor below chat; with no size set, chat="" and editor renders ""
	// so the separator newline is present
	assert.Equal(t, "\n", m.View())
}

func TestModel_Init(t *testing.T) {
	m := newModel(nil, nil)
	cmd := m.Init()
	assert.Nil(t, cmd)
}
