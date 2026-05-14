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

// UIDialogs provides modal interaction methods.
type UIDialogs interface {
	Select(title string, items []string) (int, error)
	Confirm(message string) (bool, error)
	Input(prompt string) (string, error)
	MultiSelect(title string, items []string) ([]int, error)
	Editor(prompt, initial string) (string, error)
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

type ToolRenderer interface {
	Render(content string, width int) string
}
