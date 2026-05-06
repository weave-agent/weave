package sdk

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
