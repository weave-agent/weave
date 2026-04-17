package sdk

type Bus interface {
	Publish(Event)
	Subscribe(topics ...string) <-chan Event
	SubscribeAll() <-chan Event
}

type Extension interface {
	Name() string
	Subscribe(bus Bus)
}

type ExtensionFunc struct {
	name string
	fn   func(Bus)
}

func NewExtensionFunc(name string, fn func(Bus)) *ExtensionFunc {
	return &ExtensionFunc{name: name, fn: fn}
}

func (e *ExtensionFunc) Name() string      { return e.name }
func (e *ExtensionFunc) Subscribe(bus Bus) { e.fn(bus) }
