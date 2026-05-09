package sandboxui

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUI records calls made to the sdk.UI interface.
type mockUI struct {
	statuses  map[string]string
	bindings  []sdk.Keybinding
	commands  map[string]func(string) error
	renderers map[string]sdk.ToolRenderer
}

func newMockUI() *mockUI {
	return &mockUI{
		statuses:  make(map[string]string),
		commands:  make(map[string]func(string) error),
		renderers: make(map[string]sdk.ToolRenderer),
	}
}

func (m *mockUI) Select(title string, items []string) (int, error) { return -1, nil }
func (m *mockUI) Confirm(message string) (bool, error)             { return false, nil }
func (m *mockUI) Input(prompt string) (string, error)              { return "", nil }
func (m *mockUI) SetStatus(key, text string)                       { m.statuses[key] = text }
func (m *mockUI) Notify(message string)                            {}
func (m *mockUI) RegisterCommand(name string, handler func(args string) error) {
	m.commands[name] = handler
}

func (m *mockUI) RegisterRenderer(toolName string, renderer sdk.ToolRenderer) {
	m.renderers[toolName] = renderer
}

func (m *mockUI) RegisterKeybinding(kb sdk.Keybinding) {
	m.bindings = append(m.bindings, kb)
}

// mockSandboxer implements sdk.Sandboxer for testing.
type mockSandboxer struct {
	mode string
}

func (m *mockSandboxer) WrapCommand(cmd, dir string) (string, error) { return cmd, nil }
func (m *mockSandboxer) AllowWrite(path string) bool                 { return true }
func (m *mockSandboxer) AllowRead(path string) bool                  { return true }
func (m *mockSandboxer) Mode() string                                { return m.mode }
func (m *mockSandboxer) SetMode(mode string)                         { m.mode = mode }

func TestSandboxUI_Name(t *testing.T) {
	s := &SandboxUI{}
	assert.Equal(t, "sandbox-ui", s.Name())
}

func TestSandboxUI_Register_SetsStatus(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	sdk.SetSandboxer(&mockSandboxer{mode: "auto"})

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	assert.Equal(t, "SB:auto", ui.statuses["sandbox"])
}

func TestSandboxUI_Register_SetsStatusNoSandboxer(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	sdk.SetSandboxer(nil)

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	assert.Equal(t, "SB:off", ui.statuses["sandbox"])
}

func TestSandboxUI_Register_Keybinding(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	sdk.SetSandboxer(&mockSandboxer{mode: "auto"})

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	require.Len(t, ui.bindings, 1)
	assert.Equal(t, "sandbox.cycle", ui.bindings[0].Name)
	assert.Equal(t, []string{"ctrl+s"}, ui.bindings[0].Keys)
	assert.Equal(t, "Cycle sandbox mode", ui.bindings[0].Description)
}

func TestCurrentMode_NoSandboxer(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	sdk.SetSandboxer(nil)
	assert.Equal(t, sdk.SandboxOff, currentMode())
}

func TestCurrentMode_WithSandboxer(t *testing.T) {
	defer sdk.SetSandboxer(nil)

	sdk.SetSandboxer(&mockSandboxer{mode: "ask"})
	assert.Equal(t, "ask", currentMode())
}
