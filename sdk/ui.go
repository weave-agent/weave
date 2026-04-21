package sdk

//go:generate moq -fmt goimports -out ui_mock_test.go . UI

type UI interface {
	Select(title string, items []string) (int, error)
	Confirm(message string) (bool, error)
	Input(prompt string) (string, error)
	SetStatus(key, text string)
	Notify(message string)
	RegisterCommand(name string, handler func(args string) error)
	RegisterRenderer(toolName string, renderer ToolRenderer)
	RegisterKeybinding(kb Keybinding)
}

type Keybinding struct {
	Name        string
	Keys        []string
	Description string
}

type ToolRenderer interface {
	Render(content string, width int) string
}
