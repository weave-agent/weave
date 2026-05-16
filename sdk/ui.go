package sdk

//go:generate moq -fmt goimports -out ui_mock_test.go . UI

// NotifyLevel for typed notifications.
type NotifyLevel int

const (
	NotifyInfo NotifyLevel = iota
	NotifyWarning
	NotifyError
	NotifySuccess
)

// SelectConfig holds options for Select overlay calls.
type SelectConfig struct {
	KeepContent bool
}

// ConfirmConfig holds options for Confirm overlay calls.
type ConfirmConfig struct {
	KeepContent bool
}

// InputConfig holds options for Input overlay calls.
type InputConfig struct {
	KeepContent bool
}

// EditorConfig holds options for Editor overlay calls.
type EditorConfig struct {
	KeepContent bool
}

// SelectOption is a functional option for Select.
type SelectOption func(*SelectConfig)

// ConfirmOption is a functional option for Confirm.
type ConfirmOption func(*ConfirmConfig)

// InputOption is a functional option for Input.
type InputOption func(*InputConfig)

// EditorOption is a functional option for Editor.
type EditorOption func(*EditorConfig)

// WithKeepContent docks the overlay at the bottom of the chat area
// instead of as a centered modal, keeping chat content visible.
func WithKeepContent() SelectOption {
	return func(c *SelectConfig) { c.KeepContent = true }
}

// WithKeepContentConfirm docks the confirm overlay at the bottom.
func WithKeepContentConfirm() ConfirmOption {
	return func(c *ConfirmConfig) { c.KeepContent = true }
}

// WithKeepContentInput docks the input overlay at the bottom.
func WithKeepContentInput() InputOption {
	return func(c *InputConfig) { c.KeepContent = true }
}

// WithKeepContentEditor docks the editor overlay at the bottom.
func WithKeepContentEditor() EditorOption {
	return func(c *EditorConfig) { c.KeepContent = true }
}

// UIDialogs provides modal interaction methods.
type UIDialogs interface {
	Select(title string, items []string, opts ...SelectOption) (int, error)
	Confirm(message string, opts ...ConfirmOption) (bool, error)
	Input(prompt string, opts ...InputOption) (string, error)
	MultiSelect(title string, items []string, defaults []bool, opts ...SelectOption) ([]int, error)
	Editor(prompt, initial string, opts ...EditorOption) (string, error)
}

// UIStatus provides status and notification methods.
type UIStatus interface {
	SetStatus(key, text string)
	Notify(message string)
	NotifyTyped(message string, level NotifyLevel)
	ShowError(message string)
	SetWorking(message string)
	ClearWorking()
}

// UIRegistry provides registration methods for UI extensions.
type UIRegistry interface {
	RegisterCommand(name string, handler func(args string) error)
	RegisterRenderer(toolName string, renderer ToolRenderer)
	RegisterKeybinding(kb Keybinding)
	SetTheme(name string) error
	ListThemes() []string
}

// UI is the full interface for UI implementations.
// Composed of UIDialogs + UIStatus + UIRegistry for backward compatibility.
type UI interface {
	UIDialogs
	UIStatus
	UIRegistry
}

type Keybinding struct {
	Name        string
	Keys        []string
	Description string
}

// ThemeInfo provides read-only theme information for extensions.
type ThemeInfo struct {
	Name                  string
	Accent                string
	AccentDim             string
	AccentBright          string
	Success               string
	Error                 string
	Warning               string
	Muted                 string
	MutedBright           string
	Border                string
	BorderFocused         string
	Foreground            string
	ForegroundBright      string
	BackgroundTint        string
	BackgroundTintPending string
	BackgroundTintSuccess string
	BackgroundTintError   string
}

type ToolRenderer interface {
	Render(content string, width int) string
}

// MessageRenderer renders custom message content with theme access.
type MessageRenderer interface {
	Render(content string, theme ThemeInfo, width int) string
}
