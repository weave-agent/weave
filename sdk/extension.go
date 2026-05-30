package sdk

import "fmt"

//go:generate moq -fmt goimports -stub -out extension_mock_test.go . Bus Extension

// Handler processes a bus event. Return a non-nil error to trigger an
// "extension.error" diagnostic event. Panics are caught by the bus and
// trigger an "extension.panic" diagnostic event.
type Handler func(Event) error

type Bus interface {
	Publish(Event)
	On(topic string, h Handler)
	OnAll(h Handler)
	Off(h Handler)
	Close() error
}

type Extension interface {
	Name() string
	Subscribe(bus Bus) error
	Close() error
}

type legacyRuntimeExtension struct {
	ext Extension
}

func NewLegacyRuntimeExtension(ext Extension) RuntimeExtension {
	return legacyRuntimeExtension{ext: ext}
}

func (e legacyRuntimeExtension) LegacyExtension() Extension {
	return e.ext
}

func (e legacyRuntimeExtension) Register(ctx ExtensionContext) error {
	if e.ext == nil {
		return nil
	}

	if err := e.ext.Subscribe(ctx.Bus()); err != nil {
		return fmt.Errorf("register legacy extension: %w", err)
	}

	return nil
}

func (e legacyRuntimeExtension) Close() error {
	if e.ext == nil {
		return nil
	}

	if err := e.ext.Close(); err != nil {
		return fmt.Errorf("close legacy extension: %w", err)
	}

	return nil
}

type ExtensionFunc struct {
	name    string
	fn      func(Bus) error
	closeFn func() error
}

func NewExtensionFunc(name string, fn func(Bus) error) *ExtensionFunc {
	return &ExtensionFunc{name: name, fn: fn}
}

func NewExtensionFuncWithClose(name string, fn func(Bus) error, closeFn func() error) *ExtensionFunc {
	return &ExtensionFunc{name: name, fn: fn, closeFn: closeFn}
}

func (e *ExtensionFunc) Name() string { return e.name }
func (e *ExtensionFunc) Subscribe(bus Bus) error {
	if e.fn != nil {
		return e.fn(bus)
	}

	return nil
}

func (e *ExtensionFunc) Close() error {
	if e.closeFn != nil {
		return e.closeFn()
	}

	return nil
}

type RuntimeExtensionFunc struct {
	fn      func(ExtensionContext) error
	closeFn func() error
}

func NewRuntimeExtensionFunc(fn func(ExtensionContext) error) *RuntimeExtensionFunc {
	return &RuntimeExtensionFunc{fn: fn}
}

func NewRuntimeExtensionFuncWithClose(fn func(ExtensionContext) error, closeFn func() error) *RuntimeExtensionFunc {
	return &RuntimeExtensionFunc{fn: fn, closeFn: closeFn}
}

func (e *RuntimeExtensionFunc) Register(ctx ExtensionContext) error {
	if e.fn != nil {
		return e.fn(ctx)
	}

	return nil
}

func (e *RuntimeExtensionFunc) Close() error {
	if e.closeFn != nil {
		return e.closeFn()
	}

	return nil
}

type runtimeExtensionAdapter struct {
	name    string
	cfg     Config
	runtime RuntimeExtension
}

func newRuntimeExtensionAdapter(name string, cfg Config, runtime RuntimeExtension) Extension {
	return &runtimeExtensionAdapter{name: name, cfg: cfg, runtime: runtime}
}

func (e *runtimeExtensionAdapter) Name() string { return e.name }

func (e *runtimeExtensionAdapter) Subscribe(bus Bus) error {
	if e.runtime == nil {
		return nil
	}

	if err := e.runtime.Register(NewExtensionContext(RuntimeContextOptions{
		Bus:    bus,
		Config: e.cfg,
	})); err != nil {
		return fmt.Errorf("subscribe runtime extension %q: %w", e.name, err)
	}

	return nil
}

func (e *runtimeExtensionAdapter) Close() error {
	if closer, ok := e.runtime.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close runtime extension %q: %w", e.name, err)
		}
	}

	return nil
}
