package sdk

// LifecycleHandler is called when the app reaches a lifecycle milestone.
type LifecycleHandler func(Bus, Config)

var appStartedHandlers []LifecycleHandler

// OnAppStarted registers a handler to be called when the app starts.
// Handlers run in goroutines before extension Subscribe calls.
func OnAppStarted(fn LifecycleHandler) {
	appStartedHandlers = append(appStartedHandlers, fn)
}

// AppStartedHandlers returns the registered app.started handlers.
func AppStartedHandlers() []LifecycleHandler {
	return appStartedHandlers
}

// ResetAppStartedHandlers clears all registered app.started handlers.
// For testing only.
func ResetAppStartedHandlers() {
	appStartedHandlers = nil
}
