package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopUI_Select(t *testing.T) {
	ui := NoopUI{}

	idx, err := ui.Select("pick one", []string{"a", "b", "c"})
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
}

func TestNoopUI_SelectEmpty(t *testing.T) {
	ui := NoopUI{}

	idx, err := ui.Select("pick one", []string{})
	require.NoError(t, err)
	assert.Equal(t, -1, idx)
}

func TestNoopUI_Confirm(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Confirm("are you sure?")
	require.NoError(t, err)
	assert.True(t, result)
}

func TestNoopUI_Input(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Input("enter value")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoopUI_MultiSelect(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.MultiSelect("pick some", []string{"a", "b"}, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoopUI_Editor(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Editor("edit", "initial")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoopUI_SelectWithOptions(t *testing.T) {
	ui := NoopUI{}

	idx, err := ui.Select("pick one", []string{"a", "b", "c"}, WithKeepContent())
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
}

func TestNoopUI_ConfirmWithOptions(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Confirm("are you sure?", WithKeepContentConfirm())
	require.NoError(t, err)
	assert.True(t, result)
}

func TestNoopUI_InputWithOptions(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Input("enter value", WithKeepContentInput())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoopUI_EditorWithOptions(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Editor("edit", "initial", WithKeepContentEditor())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoopUI_SetStatus(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.SetStatus("key", "value")
	})
}

func TestNoopUI_Notify(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.Notify("hello")
	})
}

func TestNoopUI_NotifyTyped(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.NotifyTyped("hello", NotifyWarning)
	})
}

func TestNoopUI_ShowError(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.ShowError("oops")
	})
}

func TestNoopUI_SetWorking(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.SetWorking("busy")
	})
}

func TestNoopUI_ClearWorking(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.ClearWorking()
	})
}

func TestNoopUI_RegisterCommand(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.RegisterCommand("test", func(args string) error { return nil })
	})
}

func TestNoopUI_RegisterRenderer(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.RegisterRenderer("tool", nil)
	})
}

func TestNoopUI_RegisterKeybinding(t *testing.T) {
	ui := NoopUI{}

	assert.NotPanics(t, func() {
		ui.RegisterKeybinding(Keybinding{
			Name:        "test",
			Keys:        []string{"ctrl+t"},
			Description: "test binding",
		})
	})
}

func TestNoopUI_SetTheme(t *testing.T) {
	ui := NoopUI{}

	err := ui.SetTheme("dark")
	require.NoError(t, err)
}

func TestNoopUI_ListThemes(t *testing.T) {
	ui := NoopUI{}

	result := ui.ListThemes()
	assert.Nil(t, result)
}
