package sandboxui

import (
	"weave/extensions/sandbox"
	"weave/sdk"
)

var currentSandboxer sdk.Sandboxer

func init() {
	sdk.OnBusReady(func(bus sdk.Bus) {
		bus.On("sandbox.registered", func(ev sdk.Event) error {
			if s, ok := ev.Payload.(sdk.Sandboxer); ok {
				currentSandboxer = s
			}

			return nil
		})
	})

	sdk.RegisterUIExtension("sandbox-ui", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.UIExtension, error) {
		return &SandboxUI{}, nil
	})
}

// SandboxUI is a TUI extension that displays the sandbox mode indicator,
// provides a keybinding to cycle between modes, and handles ask-mode
// approval dialogs.
type SandboxUI struct {
	dialog ApproveDialog
}

// Name returns the extension name, matching the directory used by AutoDiscover.
func (s *SandboxUI) Name() string { return "sandbox-ui" }

// Register wires the sandbox mode indicator and keybinding into the TUI.
func (s *SandboxUI) Register(ui sdk.UI) {
	mode := currentMode()
	ui.SetStatus("sandbox", "SB:"+mode)

	ui.RegisterKeybinding(sdk.Keybinding{
		Name:        "sandbox.cycle",
		Keys:        []string{"ctrl+s"},
		Description: "Cycle sandbox mode",
	})
}

// RegisterWithBus wires the approve dialog to the event bus.
// Implements sdk.UIExtensionWithBus.
func (s *SandboxUI) RegisterWithBus(ui sdk.UI, bus sdk.Bus) {
	s.dialog.RegisterWithBus(ui, bus)
}

// currentMode returns the active sandbox mode from the cached Sandboxer.
func currentMode() string {
	if currentSandboxer == nil {
		return sandbox.SandboxOff
	}

	return currentSandboxer.Mode()
}
