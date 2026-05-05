package sdk

import (
	"log"
	"os"

	"weave/sdk/registry"
)

var uiExtReg = registry.New[UIExtension](
	registry.WithWarn[UIExtension](log.New(os.Stderr, "weave: ", 0), "ui extension"),
)

func RegisterUIExtension(ext UIExtension) {
	uiExtReg.Register(ext.Name(), ext)
}

// UIExtensionRegistered reports whether a UI extension with the given name
// is registered.
func UIExtensionRegistered(name string) bool {
	return uiExtReg.Exists(name)
}

func GetUIExtensions() []UIExtension {
	return uiExtReg.All()
}

func ResetUIExtensionRegistry() {
	uiExtReg.Reset()
}
