package agent

import (
	"testing"

	"weave/bus"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRegistries() {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetToolRegistry()
}

func TestNewAgentExtension(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.Equal(t, "agent", ext.Name())
}

func TestAgentExtension_Close(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.NoError(t, ext.Close())
}

func TestAgentExtension_SubscribeAndClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())
}

func TestAgentExtension_SubscribeTwiceWithoutClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))

	err = ext.Subscribe(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Subscribe called twice without Close")

	require.NoError(t, ext.Close())
}

func TestAgentExtension_ReSubscribeAfterClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())
}

func TestAgentExtension_RegisterAsExtension(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterExtension("agent", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
		return NewAgentExtension(cfg)
	})

	ext, err := sdk.GetExtension("agent", nil)
	require.NoError(t, err, "GetExtension(agent)")
	assert.Equal(t, "agent", ext.Name())

	_, ok := ext.(*AgentExtension)
	require.True(t, ok, "expected *AgentExtension, got %T", ext)
}
