package sandboxui

import (
	"weave/sdk"
)

func init() {
	sdk.RegisterUIExtension(&SandboxUI{})
}

// SandboxUI is a TUI extension that displays the sandbox mode indicator,
// provides a keybinding to cycle between modes, and handles ask-mode
// approval dialogs.
type SandboxUI struct {
	dialog ApproveDialog
}

// Name returns the extension name.
func (s *SandboxUI) Name() string { return "sandbox" }

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

// currentMode returns the active sandbox mode from the global Sandboxer.
func currentMode() string {
	sb := sdk.GetSandboxer()
	if sb == nil {
		return sdk.SandboxOff
	}

	return sb.Mode()
}

// NextMode returns the next mode in the cycle order.
func NextMode(current string) string {
	return sdk.NextSandboxMode(current)
}
