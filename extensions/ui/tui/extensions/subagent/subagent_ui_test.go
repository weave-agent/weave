package subagent

import (
	"testing"
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
)

// mockTUIExtAPI records calls made to the TUIExtAPI interface.
type mockTUIExtAPI struct {
	richRenderers map[string]tui.RichToolRenderer
	panelsShown   []tui.PanelConfig
	panelsRemoved []string
}

func newMockTUIExtAPI() *mockTUIExtAPI {
	return &mockTUIExtAPI{
		richRenderers: make(map[string]tui.RichToolRenderer),
	}
}

func (m *mockTUIExtAPI) ShowPanel(config tui.PanelConfig, drawer tui.PanelDrawer) {
	m.panelsShown = append(m.panelsShown, config)
}
func (m *mockTUIExtAPI) HidePanel(id string)                                      {}
func (m *mockTUIExtAPI) RemovePanel(id string)                                    { m.panelsRemoved = append(m.panelsRemoved, id) }
func (m *mockTUIExtAPI) PanelVisible(id string) bool                              { return false }
func (m *mockTUIExtAPI) PanelTray() tui.PanelTrayAPI                              { return nil }
func (m *mockTUIExtAPI) Theme() sdk.ThemeInfo                                     { return sdk.ThemeInfo{} }
func (m *mockTUIExtAPI) Size() (int, int)                                         { return 80, 24 }
func (m *mockTUIExtAPI) EditorText() string                                       { return "" }
func (m *mockTUIExtAPI) SetEditorText(text string)                                {}
func (m *mockTUIExtAPI) PasteToEditor(text string)                                {}
func (m *mockTUIExtAPI) RegisterRichRenderer(tool string, renderer tui.RichToolRenderer) {
	m.richRenderers[tool] = renderer
}
func (m *mockTUIExtAPI) RegisterMessageRenderer(msgType string, renderer sdk.MessageRenderer) {}
func (m *mockTUIExtAPI) SetFooter(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) SetHeader(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) OnTerminalInput(handler func(tui.KeyEvent))                           {}
func (m *mockTUIExtAPI) AddAutocomplete(provider tui.AutocompleteProvider)                    {}
func (m *mockTUIExtAPI) SetWorkingFrames(frames []string, interval time.Duration)             {}
func (m *mockTUIExtAPI) RegisterTheme(name string, theme tui.ThemeDef) error                  { return nil }

func TestSubagentExtension_Name(t *testing.T) {
	ext := &SubagentExtension{}
	assert.Equal(t, "subagent", ext.Name())
}

func TestSubagentExtension_RegisterTUI_NoPanics(t *testing.T) {
	ext := &SubagentExtension{}
	api := newMockTUIExtAPI()

	assert.NotPanics(t, func() {
		ext.RegisterTUI(api)
	})
}

func TestSubagentExtension_RegisterTUI_NoRegistrations(t *testing.T) {
	ext := &SubagentExtension{}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)

	assert.Empty(t, api.richRenderers)
	assert.Empty(t, api.panelsShown)
	assert.Empty(t, api.panelsRemoved)
}
