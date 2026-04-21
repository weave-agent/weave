package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopUI_Select(t *testing.T) {
	ui := NoopUI{}

	idx, err := ui.Select("pick one", []string{"a", "b", "c"})
	assert.NoError(t, err)
	assert.Equal(t, 0, idx)
}

func TestNoopUI_Confirm(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Confirm("are you sure?")
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestNoopUI_Input(t *testing.T) {
	ui := NoopUI{}

	result, err := ui.Input("enter value")
	assert.NoError(t, err)
	assert.Equal(t, "", result)
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
			Handler:     func() error { return nil },
		})
	})
}
