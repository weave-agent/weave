package tui

import (
	"fmt"

	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"

	tea "github.com/charmbracelet/bubbletea"
)

// Dialog IDs for built-in overlays.
const (
	dialogSessionSelect  = "session-select"
	dialogModelSelect    = "model-select"
	dialogProviderSelect = "provider-select"
	dialogKeyInput       = "key-input"
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

// nextPopupDialogID generates a unique ID for popup dialog instances.
func nextPopupDialogID(kind overlayRequestKind, seq *int) string {
	*seq++

	var prefix string

	switch kind {
	case requestSelect:
		prefix = "popup-select"
	case requestConfirm:
		prefix = "popup-confirm"
	case requestInput:
		prefix = "popup-input"
	}

	return fmt.Sprintf("%s-%d", prefix, *seq)
}

// pushPopupDialog creates a dialog for a popup request and pushes it onto the stack.
func pushPopupDialog(m Model, req *overlayRequest) (Model, tea.Cmd) {
	id := nextPopupDialogID(req.kind, &m.popupSeq)
	m.popupChans[id] = req.result

	switch req.kind {
	case requestSelect:
		items := make([]overlays.SelectorItem, len(req.items))
		for i, title := range req.items {
			items[i] = overlays.SelectorItem{Title: title}
		}

		sel := overlays.NewSelectorModel(req.title, items)
		sel = sel.SetSize(m.width, m.height)
		sel = sel.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(id, sel))

	case requestConfirm:
		conf := overlays.NewConfirmModel(req.message)
		conf = conf.SetSize(m.width, m.height)
		conf = conf.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(id, conf))

	case requestInput:
		input := overlays.NewInputModel(req.message)
		input = input.SetSize(m.width, m.height)
		input = input.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(id, input))
	}

	return m, nil
}
