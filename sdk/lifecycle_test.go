package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnAppStarted(t *testing.T) {
	ResetAppStartedHandlers()
	defer ResetAppStartedHandlers()

	var called bool

	OnAppStarted(func(Bus, Config) {
		called = true
	})

	handlers := AppStartedHandlers()
	assert.Len(t, handlers, 1)

	// Invoke the handler directly to verify it was registered.
	handlers[0](nil, nil)
	assert.True(t, called)
}

func TestAppStartedHandlers_Multiple(t *testing.T) {
	ResetAppStartedHandlers()
	defer ResetAppStartedHandlers()

	var count int

	OnAppStarted(func(Bus, Config) { count++ })
	OnAppStarted(func(Bus, Config) { count++ })

	assert.Len(t, AppStartedHandlers(), 2)
}

func TestResetAppStartedHandlers(t *testing.T) {
	OnAppStarted(func(Bus, Config) {})
	ResetAppStartedHandlers()
	assert.Empty(t, AppStartedHandlers())
}
