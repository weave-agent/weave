package sandboxui

import (
	"testing"

	sandbox "github.com/weave-agent/weave-sandbox"
	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUI records calls made to the sdk.UI interface.
type mockUI struct {
	statuses         map[string]string
	bindings         []sdk.Keybinding
	commands         map[string]func(string) error
	renderers        map[string]sdk.ToolRenderer
	notifyTypedCalls []notifyTypedCall
}

type notifyTypedCall struct {
	message string
	level   sdk.NotifyLevel
}

func newMockUI() *mockUI {
	return &mockUI{
		statuses:         make(map[string]string),
		commands:         make(map[string]func(string) error),
		renderers:        make(map[string]sdk.ToolRenderer),
		notifyTypedCalls: make([]notifyTypedCall, 0),
	}
}

func (m *mockUI) Select(title string, items []string, _ ...sdk.SelectOption) (int, error) {
	return -1, nil
}

func (m *mockUI) Confirm(message string, _ ...sdk.ConfirmOption) (bool, error) {
	return false, nil
}

func (m *mockUI) Input(prompt string, _ ...sdk.InputOption) (string, error) {
	return "", nil
}

func (m *mockUI) MultiSelect(title string, items []string, _ []bool, _ ...sdk.SelectOption) ([]int, error) {
	return nil, nil
}

func (m *mockUI) Editor(prompt, initial string, _ ...sdk.EditorOption) (string, error) {
	return "", nil
}
func (m *mockUI) SetStatus(key, text string) { m.statuses[key] = text }
func (m *mockUI) Notify(message string)      {}
func (m *mockUI) NotifyTyped(message string, level sdk.NotifyLevel) {
	m.notifyTypedCalls = append(m.notifyTypedCalls, notifyTypedCall{message: message, level: level})
}
func (m *mockUI) ShowError(message string)  {}
func (m *mockUI) SetWorking(message string) {}
func (m *mockUI) ClearWorking()             {}
func (m *mockUI) RegisterCommand(name string, handler func(args string) error) {
	m.commands[name] = handler
}

func (m *mockUI) RegisterRenderer(toolName string, renderer sdk.ToolRenderer) {
	m.renderers[toolName] = renderer
}

func (m *mockUI) RegisterKeybinding(kb sdk.Keybinding) {
	m.bindings = append(m.bindings, kb)
}

func (m *mockUI) SetTheme(name string) error { return nil }
func (m *mockUI) ListThemes() []string       { return nil }

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
	defer func() { setSandboxer(nil) }()

	setSandboxer(&mockSandboxer{mode: "auto"})

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	assert.Equal(t, "SB:auto", ui.statuses["sandbox"])
}

func TestSandboxUI_Register_SetsStatusNoSandboxer(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	setSandboxer(nil)

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	assert.Equal(t, "SB:off", ui.statuses["sandbox"])
}

func TestSandboxUI_Register_Keybinding(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	setSandboxer(&mockSandboxer{mode: "auto"})

	s := &SandboxUI{}
	ui := newMockUI()

	s.Register(ui)

	require.Len(t, ui.bindings, 1)
	assert.Equal(t, "sandbox.cycle", ui.bindings[0].Name)
	assert.Equal(t, []string{"ctrl+s"}, ui.bindings[0].Keys)
	assert.Equal(t, "Cycle sandbox mode", ui.bindings[0].Description)
}

func TestCurrentMode_NoSandboxer(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	setSandboxer(nil)

	assert.Equal(t, sandbox.SandboxOff, currentMode())
}

func TestCurrentMode_WithSandboxer(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	setSandboxer(&mockSandboxer{mode: "ask"})

	assert.Equal(t, "ask", currentMode())
}

func TestSandboxUI_RegisterWithBus_ModeChangeNotification(t *testing.T) {
	ui := newMockUI()
	bus := newMockBus()

	s := &SandboxUI{}
	s.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.mode.change", "readonly"))

	assert.Equal(t, "SB:readonly", ui.statuses["sandbox"])
	require.Len(t, ui.notifyTypedCalls, 1)
	assert.Equal(t, "Sandbox mode: readonly", ui.notifyTypedCalls[0].message)
	assert.Equal(t, sdk.NotifyInfo, ui.notifyTypedCalls[0].level)
}

func TestSandboxUI_RegisterWithBus_ModeChangeInvalidPayload(t *testing.T) {
	ui := newMockUI()
	bus := newMockBus()

	s := &SandboxUI{}
	s.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.mode.change", 42))

	assert.Empty(t, ui.statuses)
	assert.Empty(t, ui.notifyTypedCalls)
}

func TestSandboxUI_RegisterWithBus_ModeChangeUpdatesStatus(t *testing.T) {
	ui := newMockUI()
	bus := newMockBus()

	s := &SandboxUI{}
	s.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.mode.change", "auto"))
	assert.Equal(t, "SB:auto", ui.statuses["sandbox"])

	bus.Publish(sdk.NewEvent("sandbox.mode.change", "off"))
	assert.Equal(t, "SB:off", ui.statuses["sandbox"])
}
