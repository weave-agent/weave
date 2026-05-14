package sdk

import (
	"fmt"
	"log/slog"

	"weave/sdk/registry"
)

var uiReg = registry.New[UI](
	registry.WithWarn[UI](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "ui")
	}),
)

func RegisterUI(name string, ui UI) {
	uiReg.Register(name, ui)
}

func GetUI(name string) (UI, error) {
	ui, ok := uiReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("ui %q not registered", name)
	}

	return ui, nil
}

func ListUIs() []string {
	return uiReg.List()
}

func ResetUIRegistry() {
	uiReg.Reset()
}
