package tui

import (
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

// DialogStack manages a stack of dialog overlays.
// Full implementation lives in Task 7 (overlay stack).
type DialogStack struct{}

// Dialog is the interface for overlay dialogs rendered into screen buffers.
// Full implementation lives in Task 7 (overlay stack).
type Dialog interface {
	ID() string
	Draw(scr uv.Screen, area uv.Rectangle)
}

// overlayKind identifies which inline overlay is active.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlaySession
	overlayModel
	overlayProvider
	overlayKeyInput
)

// overlayRequestKind identifies the type of cross-extension popup request.
type overlayRequestKind int

const (
	requestSelect overlayRequestKind = iota
	requestConfirm
	requestInput
)

// overlayRequest is an internal message sent to the Bubble Tea program
// to trigger a popup overlay (Select, Confirm, or Input).
type overlayRequest struct {
	kind    overlayRequestKind
	title   string
	message string
	items   []string
	result  chan overlayResponse
}

// overlayResponse carries the result back to the blocking caller.
type overlayResponse struct {
	index     int
	value     string
	confirmed bool
	err       error
}

// popupState tracks the active cross-extension popup and its response channel.
type popupState struct {
	kind    overlayRequestKind
	confirm overlays.ConfirmModel
	input   overlays.InputModel
	select_ overlays.SelectorModel
	items   []string
	result  chan overlayResponse
}

// Internal tea.Msg types.

type popupPendingMsg struct{}

type extStatusMsg struct {
	key  string
	text string
}

type notifyMsg struct {
	message string
}

// slashCommandsUpdatedMsg is sent when commands are dynamically registered,
// so the editor can refresh its autocomplete list.
type slashCommandsUpdatedMsg struct{}

// checkNextPopupCmd returns a tea.Cmd that sends popupPendingMsg
// if there are more queued popups.
func checkNextPopupCmd(ui *TUIImpl) tea.Cmd {
	if ui != nil && ui.hasPendingPopups() {
		return func() tea.Msg { return popupPendingMsg{} }
	}

	return nil
}

// newNotifyAssistantMsg creates a finalized assistant message for notifications.
func newNotifyAssistantMsg(text string) *messages.AssistantMessage {
	am := messages.NewAssistantMessage()
	am.Finalize(text)

	return am
}
