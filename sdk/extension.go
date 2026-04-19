package sdk

type Bus interface {
	Publish(Event)
	Subscribe(topics ...string) <-chan Event
	SubscribeAll() <-chan Event
}

type Extension interface {
	Name() string
	Subscribe(bus Bus)
	Close() error
}

type ExtensionFunc struct {
	name    string
	fn      func(Bus)
	closeFn func() error
}

func NewExtensionFunc(name string, fn func(Bus)) *ExtensionFunc {
	return &ExtensionFunc{name: name, fn: fn}
}

func NewExtensionFuncWithClose(name string, fn func(Bus), closeFn func() error) *ExtensionFunc {
	return &ExtensionFunc{name: name, fn: fn, closeFn: closeFn}
}

func (e *ExtensionFunc) Name() string      { return e.name }
func (e *ExtensionFunc) Subscribe(bus Bus) { e.fn(bus) }
func (e *ExtensionFunc) Close() error {
	if e.closeFn != nil {
		return e.closeFn()
	}

	return nil
}
