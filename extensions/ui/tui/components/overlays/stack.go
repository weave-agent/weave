package overlays

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// DialogResult holds the outcome of a completed dialog.
type DialogResult struct {
	Index     int
	Value     string
	Confirmed bool
	Selected  []int
	Err       error
}

// Dialog is the interface for overlay dialogs rendered into screen buffers.
type Dialog interface {
	// ID returns a unique identifier for this dialog instance.
	ID() string
	// Update handles a message. Returns the (possibly modified) dialog and a command.
	Update(msg tea.Msg) (Dialog, tea.Cmd)
	// Draw renders the dialog into a screen buffer region.
	Draw(scr uv.Screen, area uv.Rectangle)
	// Handles returns true if this dialog wants to handle the given message.
	Handles(msg tea.Msg) bool
	// Done returns true if the dialog has completed and should be removed.
	Done() bool
	// Result returns the dialog's outcome. Only valid after Done() returns true.
	Result() DialogResult
	// SetSize updates the dialog's dimensions for centering calculations.
	SetSize(width, height int) Dialog
}

// DialogStack manages a stack of dialog overlays.
// Dialogs are rendered bottom-to-top and key events are routed top-to-bottom
// with fall-through for unhandled messages.
type DialogStack struct {
	dialogs []Dialog
}

// NewDialogStack creates a new empty dialog stack.
func NewDialogStack() DialogStack {
	return DialogStack{}
}

// Push adds a dialog to the top of the stack.
func (s DialogStack) Push(d Dialog) DialogStack {
	s.dialogs = append(s.dialogs, d)
	return s
}

// Pop removes and returns the top dialog. Returns the stack and nil if empty.
func (s DialogStack) Pop() (DialogStack, Dialog) {
	if len(s.dialogs) == 0 {
		return s, nil
	}

	top := s.dialogs[len(s.dialogs)-1]
	s.dialogs = s.dialogs[:len(s.dialogs)-1]

	return s, top
}

// Peek returns the top dialog without removing it. Returns nil if empty.
func (s DialogStack) Peek() Dialog {
	if len(s.dialogs) == 0 {
		return nil
	}

	return s.dialogs[len(s.dialogs)-1]
}

// Empty returns true if the stack has no dialogs.
func (s DialogStack) Empty() bool {
	return len(s.dialogs) == 0
}

// Len returns the number of dialogs on the stack.
func (s DialogStack) Len() int {
	return len(s.dialogs)
}

// Update routes messages through the dialog stack.
// Key events are routed top-to-bottom with fall-through.
// Non-key messages are only sent to the top dialog.
// Returns the updated stack, a command, and any dialogs that completed
// during fall-through (so the caller can process their results).
func (s DialogStack) Update(msg tea.Msg) (DialogStack, tea.Cmd, []Dialog) {
	if s.Empty() {
		return s, nil, nil
	}

	// Route to top dialog first.
	top := s.Peek()
	if top.Handles(msg) {
		newDialog, cmd := top.Update(msg)
		s.dialogs[len(s.dialogs)-1] = newDialog

		return s, cmd, nil
	}

	// Fall-through for key events to lower dialogs.
	if _, ok := msg.(tea.KeyPressMsg); ok {
		for i := len(s.dialogs) - 2; i >= 0; i-- {
			if !s.dialogs[i].Handles(msg) {
				continue
			}

			newDialog, cmd := s.dialogs[i].Update(msg)
			s.dialogs[i] = newDialog

			var completed []Dialog

			if newDialog.Done() {
				completed = append(completed, newDialog)
				s.dialogs = append(s.dialogs[:i], s.dialogs[i+1:]...)
			}

			return s, cmd, completed
		}
	}

	return s, nil, nil
}

// Draw renders all dialogs bottom-to-top into the screen buffer.
func (s DialogStack) Draw(scr uv.Screen, area uv.Rectangle) {
	for _, d := range s.dialogs {
		d.Draw(scr, area)
	}
}

// Resize updates the dimensions of all dialogs on the stack.
func (s DialogStack) Resize(width, height int) DialogStack {
	for i, d := range s.dialogs {
		s.dialogs[i] = d.SetSize(width, height)
	}

	return s
}

// --- Selector Dialog Adapter ---

// SelectorDialog wraps a SelectorModel as a Dialog.
type SelectorDialog struct {
	id     string
	model  SelectorModel
	done   bool
	result DialogResult
}

// NewSelectorDialog creates a dialog wrapping a SelectorModel.
func NewSelectorDialog(id string, model SelectorModel) *SelectorDialog {
	return &SelectorDialog{id: id, model: model}
}

func (d *SelectorDialog) ID() string           { return d.id }
func (d *SelectorDialog) Done() bool           { return d.done }
func (d *SelectorDialog) Result() DialogResult { return d.result }
func (d *SelectorDialog) Model() SelectorModel { return d.model }

func (d *SelectorDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *SelectorDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case SelectorSelectedMsg:
		d.done = true
		d.result = DialogResult{Index: msg.Index}

		return d, nil

	case SelectorCancelledMsg:
		d.done = true
		d.result = DialogResult{Index: -1, Err: errors.New("canceled")}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *SelectorDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *SelectorDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case SelectorSelectedMsg, SelectorCancelledMsg:
		return true
	}

	return false
}

// --- Confirm Dialog Adapter ---

// ConfirmDialog wraps a ConfirmModel as a Dialog.
type ConfirmDialog struct {
	id     string
	model  ConfirmModel
	done   bool
	result DialogResult
}

// NewConfirmDialog creates a dialog wrapping a ConfirmModel.
func NewConfirmDialog(id string, model ConfirmModel) *ConfirmDialog {
	return &ConfirmDialog{id: id, model: model}
}

func (d *ConfirmDialog) ID() string           { return d.id }
func (d *ConfirmDialog) Done() bool           { return d.done }
func (d *ConfirmDialog) Result() DialogResult { return d.result }
func (d *ConfirmDialog) Model() ConfirmModel  { return d.model }

func (d *ConfirmDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *ConfirmDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case ConfirmResultMsg:
		d.done = true
		d.result = DialogResult{Confirmed: msg.Confirmed}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *ConfirmDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *ConfirmDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case ConfirmResultMsg:
		return true
	}

	return false
}

// --- Input Dialog Adapter ---

// InputDialog wraps an InputModel as a Dialog.
type InputDialog struct {
	id     string
	model  InputModel
	done   bool
	result DialogResult
}

// NewInputDialog creates a dialog wrapping an InputModel.
func NewInputDialog(id string, model InputModel) *InputDialog {
	return &InputDialog{id: id, model: model}
}

func (d *InputDialog) ID() string           { return d.id }
func (d *InputDialog) Done() bool           { return d.done }
func (d *InputDialog) Result() DialogResult { return d.result }
func (d *InputDialog) Model() InputModel    { return d.model }

func (d *InputDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *InputDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case InputResultMsg:
		d.done = true

		if msg.Ok {
			d.result = DialogResult{Value: msg.Value}
		} else {
			d.result = DialogResult{Err: errors.New("canceled")}
		}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *InputDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *InputDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case InputResultMsg:
		return true
	}

	return false
}

// --- Editor Dialog Adapter ---

// EditorDialog wraps an EditorModel as a Dialog.
type EditorDialog struct {
	id     string
	model  EditorModel
	done   bool
	result DialogResult
}

// NewEditorDialog creates a dialog wrapping an EditorModel.
func NewEditorDialog(id string, model EditorModel) *EditorDialog {
	return &EditorDialog{id: id, model: model}
}

func (d *EditorDialog) ID() string           { return d.id }
func (d *EditorDialog) Done() bool           { return d.done }
func (d *EditorDialog) Result() DialogResult { return d.result }
func (d *EditorDialog) Model() EditorModel   { return d.model }

func (d *EditorDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *EditorDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case EditorResultMsg:
		d.done = true

		if msg.Ok {
			d.result = DialogResult{Value: msg.Value}
		} else {
			d.result = DialogResult{Err: errors.New("canceled")}
		}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *EditorDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *EditorDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case EditorResultMsg:
		return true
	}

	return false
}

// --- MultiSelect Dialog Adapter ---

// MultiSelectDialog wraps a MultiSelectModel as a Dialog.
type MultiSelectDialog struct {
	id     string
	model  MultiSelectModel
	done   bool
	result DialogResult
}

// NewMultiSelectDialog creates a dialog wrapping a MultiSelectModel.
func NewMultiSelectDialog(id string, model MultiSelectModel) *MultiSelectDialog {
	return &MultiSelectDialog{id: id, model: model}
}

func (d *MultiSelectDialog) ID() string              { return d.id }
func (d *MultiSelectDialog) Done() bool              { return d.done }
func (d *MultiSelectDialog) Result() DialogResult    { return d.result }
func (d *MultiSelectDialog) Model() MultiSelectModel { return d.model }

func (d *MultiSelectDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *MultiSelectDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case MultiSelectResultMsg:
		d.done = true
		if msg.Ok {
			d.result = DialogResult{Selected: msg.Selected}
		} else {
			d.result = DialogResult{Err: errors.New("canceled")}
		}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *MultiSelectDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *MultiSelectDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case MultiSelectResultMsg:
		return true
	}

	return false
}
