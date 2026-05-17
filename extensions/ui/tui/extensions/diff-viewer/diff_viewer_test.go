package diffviewer

import (
	"testing"
	"time"

	tui "github.com/weave-agent/weave-tui"
	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTUIExtAPI records calls made to the TUIExtAPI interface.
type mockTUIExtAPI struct {
	richRenderers map[string]tui.RichToolRenderer
}

func newMockTUIExtAPI() *mockTUIExtAPI {
	return &mockTUIExtAPI{
		richRenderers: make(map[string]tui.RichToolRenderer),
	}
}

func (m *mockTUIExtAPI) ShowPanel(config tui.PanelConfig, drawer tui.PanelDrawer) {}
func (m *mockTUIExtAPI) HidePanel(id string)                                      {}
func (m *mockTUIExtAPI) RemovePanel(id string)                                    {}
func (m *mockTUIExtAPI) PanelVisible(id string) bool                              { return false }
func (m *mockTUIExtAPI) PanelTray() tui.PanelTrayAPI                              { return nil }
func (m *mockTUIExtAPI) Theme() sdk.ThemeInfo                                     { return sdk.ThemeInfo{} }
func (m *mockTUIExtAPI) Size() (int, int)                                         { return 0, 0 }
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
func (m *mockTUIExtAPI) RequestRedraw()                                                       {}

func TestDiffViewer_Name(t *testing.T) {
	dv := &DiffViewer{}
	assert.Equal(t, "diff-viewer", dv.Name())
}

func TestDiffViewer_RegisterTUI(t *testing.T) {
	dv := &DiffViewer{}
	api := newMockTUIExtAPI()

	dv.RegisterTUI(api)

	renderer, ok := api.richRenderers["edit"]
	require.True(t, ok, "expected edit renderer to be registered")
	assert.NotNil(t, renderer)
}

func TestDiffViewer_RegisterTUI_NoOtherRenderers(t *testing.T) {
	dv := &DiffViewer{}
	api := newMockTUIExtAPI()

	dv.RegisterTUI(api)

	assert.Len(t, api.richRenderers, 1, "expected exactly one renderer to be registered")
	assert.Contains(t, api.richRenderers, "edit")
}

func TestRichDiffRenderer_Render(t *testing.T) {
	r := &richDiffRenderer{}

	input := `--- a/main.go
+++ b/main.go
@@ -1,5 +1,5 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }
`

	theme := sdk.ThemeInfo{
		Accent:       "63",
		AccentBright: "69",
		Success:      "84",
		Error:        "204",
		Muted:        "245",
	}

	result := r.Render(input, theme, 80)

	// Result should contain the original content (with ANSI styling)
	assert.Contains(t, result, "--- a/main.go")
	assert.Contains(t, result, "+++ b/main.go")
	assert.Contains(t, result, `package main`)
	assert.Contains(t, result, `fmt.Println("hello")`)
	assert.Contains(t, result, `fmt.Println("world")`)
}

func TestRichDiffRenderer_RenderEmpty(t *testing.T) {
	r := &richDiffRenderer{}
	result := r.Render("", sdk.ThemeInfo{}, 80)
	assert.Empty(t, result)
}

func TestRichDiffRenderer_RenderNonDiff(t *testing.T) {
	r := &richDiffRenderer{}
	input := "some plain text\nwithout diff markers"
	theme := sdk.ThemeInfo{Muted: "245"}

	result := r.Render(input, theme, 80)

	// Should still render (as faint context lines)
	assert.Contains(t, result, "some plain text")
}

func TestRichDiffRenderer_UsesThemeColors(t *testing.T) {
	r := &richDiffRenderer{}

	theme := sdk.ThemeInfo{
		Accent:       "99",
		AccentBright: "135",
		Success:      "120",
		Error:        "198",
		Muted:        "240",
	}

	input := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,2 @@
-old line
+new line
 context
`

	result := r.Render(input, theme, 80)

	// All content should be present with styling applied
	assert.Contains(t, result, "--- a/file.go")
	assert.Contains(t, result, "+++ b/file.go")
	assert.Contains(t, result, "@@ -1,2 +1,2 @@")
	assert.Contains(t, result, "-old line")
	assert.Contains(t, result, "+new line")
	assert.Contains(t, result, "context")
}
