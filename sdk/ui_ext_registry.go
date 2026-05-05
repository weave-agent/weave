package sdk

import (
	"log"
	"os"
	"sort"
	"sync"
)

var uiExtWarnLog = log.New(os.Stderr, "weave: ", 0)

var (
	uiExtMu  sync.RWMutex
	uiExtReg = make(map[string]UIExtension)
)

func RegisterUIExtension(ext UIExtension) {
	uiExtMu.Lock()
	defer uiExtMu.Unlock()

	name := ext.Name()

	if _, dup := uiExtReg[name]; dup {
		uiExtWarnLog.Printf("warning: ui extension %q already registered; first registration wins", name)
		return
	}

	uiExtReg[name] = ext
}

// UIExtensionRegistered reports whether a UI extension with the given name
// is registered.
func UIExtensionRegistered(name string) bool {
	uiExtMu.RLock()
	defer uiExtMu.RUnlock()

	_, ok := uiExtReg[name]

	return ok
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
