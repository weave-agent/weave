package tui

import (
	"fmt"

	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"

	tea "charm.land/bubbletea/v2"
)

// Dialog IDs for built-in overlays.
const (
	dialogSessionSelect  = "session-select"
	dialogModelSelect    = "model-select"
	dialogProviderSelect = "provider-select"
	dialogKeyInput       = "key-input"
	dialogLoginSelect    = "login-select"
	dialogLogoutSelect   = "logout-select"
	dialogLoginOAuth     = "login-oauth"
)

// overlayRequestKind identifies the type of cross-extension popup request.
type overlayRequestKind int

const (
	requestSelect overlayRequestKind = iota
	requestConfirm
	requestInput
	requestEditor
	requestMultiSelect
)

// overlayRequest is an internal message sent to the Bubble Tea program
// to trigger a popup overlay (Select, Confirm, Input, Editor, or MultiSelect).
type overlayRequest struct {
	kind        overlayRequestKind
	title       string
	message     string
	items       []string
	initial     string
	defaults    []bool
	keepContent bool
	mask        rune
	result      chan overlayResponse
}

// overlayResponse carries the result back to the blocking caller.
type overlayResponse struct {
	index     int
	value     string
	confirmed bool
	selected  []int
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

type notifyTypedMsg struct {
	message string
	level   sdk.NotifyLevel
}

// slashCommandsUpdatedMsg is sent when commands are dynamically registered,
// so the editor can refresh its autocomplete list.
type slashCommandsUpdatedMsg struct{}

// themeChangedMsg is sent when the active theme is switched.
type themeChangedMsg struct {
	theme *palette.Theme
}

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
	case requestEditor:
		prefix = "popup-editor"
	case requestMultiSelect:
		prefix = "popup-multiselect"
	}

	return fmt.Sprintf("%s-%d", prefix, *seq)
}

// pushPopupDialog creates a dialog for a popup request and pushes it onto the stack.
func pushPopupDialog(m Model, req *overlayRequest) (Model, tea.Cmd) {
	id := nextPopupDialogID(req.kind, &m.popupSeq)
	m.popupChans[id] = req.result
	m.dockedOverlay = req.keepContent

	dialogWidth := m.width

	dialogHeight := m.height
	if req.keepContent {
		dialogHeight = dockedOverlayHeight
	}

	switch req.kind {
	case requestSelect:
		items := make([]overlays.SelectorItem, len(req.items))
		for i, title := range req.items {
			items[i] = overlays.SelectorItem{Title: title}
		}

		sel := overlays.NewSelectorModel(req.title, items)
		sel = sel.SetSize(dialogWidth, dialogHeight)
		sel = sel.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(id, sel))

	case requestConfirm:
		conf := overlays.NewConfirmModel(req.message)
		conf = conf.SetSize(dialogWidth, dialogHeight)
		conf = conf.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(id, conf))

	case requestInput:
		input := overlays.NewInputModel(req.message)
		input = input.SetSize(dialogWidth, dialogHeight)
		input = input.SetMask(req.mask)
		input = input.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(id, input))

	case requestEditor:
		editor := overlays.NewEditorModel(req.title, req.initial)
		editor = editor.SetSize(dialogWidth, dialogHeight)
		editor = editor.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewEditorDialog(id, editor))

	case requestMultiSelect:
		ms := overlays.NewMultiSelectModel(req.title, req.items, req.defaults)
		ms = ms.SetSize(dialogWidth, dialogHeight)
		ms = ms.Show()

		m.dialogStack = m.dialogStack.Push(overlays.NewMultiSelectDialog(id, ms))
	}

	return m, nil
}
