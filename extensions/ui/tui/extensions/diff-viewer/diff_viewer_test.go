package diffviewer

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUI records calls made to the sdk.UI interface.
type mockUI struct {
	renderers map[string]sdk.ToolRenderer
	commands  map[string]func(string) error
	bindings  []sdk.Keybinding
}

func newMockUI() *mockUI {
	return &mockUI{
		renderers: make(map[string]sdk.ToolRenderer),
		commands:  make(map[string]func(string) error),
	}
}

func (m *mockUI) Select(title string, items []string) (int, error) { return -1, nil }
func (m *mockUI) Confirm(message string) (bool, error)              { return false, nil }
func (m *mockUI) Input(prompt string) (string, error)               { return "", nil }
func (m *mockUI) SetStatus(key, text string)                        {}
func (m *mockUI) Notify(message string)                             {}
func (m *mockUI) RegisterCommand(name string, handler func(args string) error) {
	m.commands[name] = handler
}
func (m *mockUI) RegisterRenderer(toolName string, renderer sdk.ToolRenderer) {
	m.renderers[toolName] = renderer
}
func (m *mockUI) RegisterKeybinding(kb sdk.Keybinding) {
	m.bindings = append(m.bindings, kb)
}

func TestDiffViewer_Name(t *testing.T) {
	dv := &DiffViewer{}
	assert.Equal(t, "diff-viewer", dv.Name())
}

func TestDiffViewer_Register(t *testing.T) {
	dv := &DiffViewer{}
	ui := newMockUI()

	dv.Register(ui)

	renderer, ok := ui.renderers["edit"]
	require.True(t, ok, "expected edit renderer to be registered")
	assert.NotNil(t, renderer)
}

func TestDiffViewer_Register_NoOtherRenderers(t *testing.T) {
	dv := &DiffViewer{}
	ui := newMockUI()

	dv.Register(ui)

	assert.Len(t, ui.renderers, 1, "expected exactly one renderer to be registered")
	assert.Contains(t, ui.renderers, "edit")
}

func TestDiffRenderer_Render(t *testing.T) {
	r := &diffRenderer{}

	input := `--- a/main.go
+++ b/main.go
@@ -1,5 +1,5 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }
`

	result := r.Render(input, 80)

	// Result should contain the original content (with ANSI styling)
	assert.Contains(t, result, "--- a/main.go")
	assert.Contains(t, result, "+++ b/main.go")
	assert.Contains(t, result, `package main`)
	assert.Contains(t, result, `fmt.Println("hello")`)
	assert.Contains(t, result, `fmt.Println("world")`)
}

func TestDiffRenderer_RenderEmpty(t *testing.T) {
	r := &diffRenderer{}
	result := r.Render("", 80)
	assert.Equal(t, "", result)
}

func TestDiffRenderer_RenderNonDiff(t *testing.T) {
	r := &diffRenderer{}
	input := "some plain text\nwithout diff markers"
	result := r.Render(input, 80)

	// Should still render (as faint context lines)
	assert.Contains(t, result, "some plain text")
}
