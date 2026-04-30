package sdk

import (
	"sort"
	"sync"
)

var (
	uiExtMu  sync.RWMutex
	uiExtReg = make(map[string]UIExtension)
)

func RegisterUIExtension(ext UIExtension) {
	uiExtMu.Lock()
	defer uiExtMu.Unlock()

	name := ext.Name()

	if _, dup := uiExtReg[name]; dup {
		panic("sdk: RegisterUIExtension called twice for " + name)
	}

	uiExtReg[name] = ext
}

func GetUIExtensions() []UIExtension {
	uiExtMu.RLock()
	defer uiExtMu.RUnlock()

	exts := make([]UIExtension, 0, len(uiExtReg))

	names := make([]string, 0, len(uiExtReg))
	for name := range uiExtReg {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		exts = append(exts, uiExtReg[name])
	}

	return exts
}

func ResetUIExtensionRegistry() {
	uiExtMu.Lock()
	defer uiExtMu.Unlock()

	uiExtReg = make(map[string]UIExtension)
}
