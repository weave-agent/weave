package sdk

import (
	"fmt"
	"log"
	"os"

	"weave/sdk/registry"
)

var uiReg = registry.New[UI](
	registry.WithWarn[UI](log.New(os.Stderr, "weave: ", 0), "ui"),
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
