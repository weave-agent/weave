package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileTimeChecks verifies that NoopUI implements all sub-interfaces and UI.
func TestNoopUI_ImplementsUIDialogs(t *testing.T) {
	var _ UIDialogs = NoopUI{}
}

func TestNoopUI_ImplementsUIStatus(t *testing.T) {
	var _ UIStatus = NoopUI{}
}

func TestNoopUI_ImplementsUIRegistry(t *testing.T) {
	var _ UIRegistry = NoopUI{}
}

func TestNoopUI_ImplementsUI(t *testing.T) {
	var _ UI = NoopUI{}
}

// TestUIComposition verifies that a type implementing all three sub-interfaces
// satisfies the composite UI interface.
func TestUIComposition(t *testing.T) {
	// NoopUI implements all three sub-interfaces, so it must satisfy UI.
	ui := UI(NoopUI{})
	assert.NotNil(t, ui)

	// Verify methods from each sub-interface are accessible via UI.
	idx, err := ui.Select("test", []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, 0, idx)

	ui.SetStatus("key", "val")
	ui.RegisterCommand("cmd", func(string) error { return nil })
}

// TestNotifyLevelConstants verifies NotifyLevel values.
func TestNotifyLevelConstants(t *testing.T) {
	assert.Equal(t, NotifyInfo, NotifyLevel(0))
	assert.Equal(t, NotifyWarning, NotifyLevel(1))
	assert.Equal(t, NotifyError, NotifyLevel(2))
	assert.Equal(t, NotifySuccess, NotifyLevel(3))
}
