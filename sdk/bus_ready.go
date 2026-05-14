package sdk

import (
	"io"
	"sync"
)

// BusReadyFunc is called when the event bus is available, allowing code that
// does not have a Subscribe method (e.g. tools) to register bus subscriptions.
type BusReadyFunc func(Bus)

var (
	busReadyFuncs   []BusReadyFunc
	busReadyFuncsMu sync.Mutex
)

// OnBusReady registers a callback that is invoked when the event bus becomes
// available during wiring. Tools and other non-extension code use this to
// subscribe to bus events (e.g. sandbox.registered, output.redirect).
func OnBusReady(fn BusReadyFunc) {
	busReadyFuncsMu.Lock()
	defer busReadyFuncsMu.Unlock()

	busReadyFuncs = append(busReadyFuncs, fn)
}

// InvokeBusSubscribers calls all registered BusReadyFunc callbacks with the
// given bus. It is called by the wiring layer before extension Subscribe
// methods run, so that tool subscriptions are active when extensions publish
// their registration events.
func InvokeBusSubscribers(bus Bus) {
	busReadyFuncsMu.Lock()
	funcs := make([]BusReadyFunc, len(busReadyFuncs))
	copy(funcs, busReadyFuncs)
	busReadyFuncsMu.Unlock()

	for _, fn := range funcs {
		fn(bus)
	}
}

// ResetBusSubscribers clears all registered BusReadyFunc callbacks.
// For testing only.
func ResetBusSubscribers() {
	busReadyFuncsMu.Lock()
	defer busReadyFuncsMu.Unlock()

	busReadyFuncs = nil
}

// OutputRedirectPayload is the payload for output.redirect bus events.
type OutputRedirectPayload struct {
	Writer io.Writer
}
