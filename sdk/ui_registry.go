package sdk

import (
	"fmt"
	"sort"
	"sync"
)

var (
	uiMu  sync.RWMutex
	uiReg = make(map[string]UI)
)

func RegisterUI(name string, ui UI) {
	uiMu.Lock()
	defer uiMu.Unlock()

	if _, dup := uiReg[name]; dup {
		panic("sdk: RegisterUI called twice for " + name)
	}

	uiReg[name] = ui
}

func GetUI(name string) (UI, error) {
	uiMu.RLock()

	ui, ok := uiReg[name]

	uiMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("ui %q not registered", name)
	}

	return ui, nil
}

func ListUIs() []string {
	uiMu.RLock()
	defer uiMu.RUnlock()

	names := make([]string, 0, len(uiReg))
	for name := range uiReg {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func ResetUIRegistry() {
	uiMu.Lock()
	defer uiMu.Unlock()

	uiReg = make(map[string]UI)
}
