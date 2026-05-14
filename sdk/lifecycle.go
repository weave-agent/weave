package sdk

import "sync"

// LifecycleHandler is called when the app reaches a lifecycle milestone.
type LifecycleHandler func(Bus, Config)

var (
	appStartedHandlers   []LifecycleHandler
	appStartedHandlersMu sync.RWMutex
)

// OnAppStarted registers a handler to be called when the app starts.
// Handlers run in goroutines before extension Subscribe calls.
func OnAppStarted(fn LifecycleHandler) {
	appStartedHandlersMu.Lock()
	defer appStartedHandlersMu.Unlock()

	appStartedHandlers = append(appStartedHandlers, fn)
}

// AppStartedHandlers returns the registered app.started handlers.
func AppStartedHandlers() []LifecycleHandler {
	appStartedHandlersMu.RLock()
	defer appStartedHandlersMu.RUnlock()

	result := make([]LifecycleHandler, len(appStartedHandlers))
	copy(result, appStartedHandlers)

	return result
}

// ResetAppStartedHandlers clears all registered app.started handlers.
// For testing only.
func ResetAppStartedHandlers() {
	appStartedHandlersMu.Lock()
	defer appStartedHandlersMu.Unlock()

	appStartedHandlers = nil
}
