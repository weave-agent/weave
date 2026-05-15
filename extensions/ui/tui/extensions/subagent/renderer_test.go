package subagent

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"weave/ext/ui/tui"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripANSI removes ANSI escape sequences from a string.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestSubagentRenderer_RenderEmpty(t *testing.T) {
	r := &subagentRenderer{}
	result := r.Render("", sdk.ThemeInfo{}, 80)
	assert.Empty(t, result)
}

func TestSubagentRenderer_RenderBackgroundResponse(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{
		Primary:       "63",
		PrimaryBright: "69",
		MutedBright:   "252",
	}

	content, _ := json.Marshal(map[string]string{
		"id":     "subagent_researcher_abc123",
		"status": "running",
	})

	result := r.Render(string(content), theme, 80)

	assert.Contains(t, result, "subagent_researcher_abc123")
	assert.Contains(t, result, "running")
}

func TestSubagentRenderer_RenderForegroundOutput(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{
		Foreground: "15",
	}

	content := "line 1\nline 2\nline 3"
	result := r.Render(content, theme, 80)

	assert.Contains(t, result, "line 1")
	assert.Contains(t, result, "line 2")
	assert.Contains(t, result, "line 3")
}

func TestSubagentRenderer_RenderForegroundOutput_Truncation(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	lines := make([]string, 15)
	for i := range lines {
		lines[i] = "some output line"
	}

	content := strings.Join(lines, "\n")

	result := r.Render(content, theme, 80)

	// Should truncate to 8 lines with a "... (N more lines)" indicator.
	assert.Contains(t, result, "... (")
	assert.Contains(t, result, "more lines)")
	// The output should be shorter than the full 15-line input.
	assert.Less(t, strings.Count(result, "some output line"), 15)
}

func TestSubagentRenderer_RenderForegroundOutput_WideLine(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	longLine := strings.Repeat("x", 120)
	result := r.Render(longLine, theme, 80)

	// Result must contain "..." and be shorter than the full 120-char input.
	assert.Contains(t, result, "...")
	stripped := stripANSI(result)
	assert.Less(t, len(stripped), 120)
}

func TestSubagentRenderer_RenderForegroundOutput_MultiByte(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	// 120 Japanese characters (3 bytes each = 360 bytes).
	longLine := strings.Repeat("日", 120)
	result := r.Render(longLine, theme, 80)

	// Must produce valid UTF-8 (byte-based slicing would break this).
	assert.True(t, utf8.ValidString(result), "result should be valid UTF-8")
	stripped := stripANSI(result)
	assert.Contains(t, stripped, "...")
}

func TestSubagentRenderer_RenderNonJSON(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	content := "This is plain text output from a foreground subagent."
	result := r.Render(content, theme, 80)

	assert.Contains(t, result, "This is plain text output")
}

func TestSubagentRenderer_RegisterTUI_RegistersBuiltInRenderers(t *testing.T) {
	ext := &SubagentExtension{
		tracker:  NewAgentTracker(gracePeriod, nil),
		renderer: &subagentRenderer{},
	}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)

	require.Len(t, api.richRenderers, 3)
	assert.Contains(t, api.richRenderers, "subagent_general")
	assert.Contains(t, api.richRenderers, "subagent_explore")
	assert.Contains(t, api.richRenderers, "subagent_plan")

	// All three should share the same renderer instance.
	renderer := api.richRenderers["subagent_general"]
	assert.NotNil(t, renderer)
	assert.Same(t, renderer, api.richRenderers["subagent_explore"])
	assert.Same(t, renderer, api.richRenderers["subagent_plan"])
}

func TestSubagentRenderer_DynamicRegistrationOnStart(t *testing.T) {
	ext := &SubagentExtension{
		tracker:  NewAgentTracker(gracePeriod, nil),
		renderer: &subagentRenderer{},
	}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// A custom agent name not in built-ins.
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id":   "custom-123",
		"name": "researcher",
		"mode": "background",
	}))

	// Renderer should now be registered for subagent_researcher.
	assert.Contains(t, api.richRenderers, "subagent_researcher")
	assert.NotNil(t, api.richRenderers["subagent_researcher"])
}

func TestSubagentRenderer_DynamicRegistration_SameRenderer(t *testing.T) {
	ext := &SubagentExtension{
		tracker:  NewAgentTracker(gracePeriod, nil),
		renderer: &subagentRenderer{},
	}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-1", "name": "general", "mode": "background",
	}))

	// Built-in "general" renderer should still be the same instance.
	assert.Same(t, ext.renderer, api.richRenderers["subagent_general"])
}

func TestSubagentRenderer_RendBackgroundResponse_ContainsIcon(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{
		Primary:       "63",
		PrimaryBright: "69",
		MutedBright:   "252",
	}

	content, _ := json.Marshal(map[string]string{
		"id":     "agent-xyz",
		"status": "running",
	})

	result := r.Render(string(content), theme, 80)
	assert.Contains(t, result, "agent-xyz")
	assert.Contains(t, result, "running")
}

func TestSubagentRenderer_RenderNonBackgroundJSON(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	// JSON that doesn't match background response pattern (no "id" field).
	content := `{"message": "hello"}`
	result := r.Render(content, theme, 80)

	// Should fall through to foreground output rendering.
	assert.Contains(t, result, "message")
}

func TestSubagentRenderer_RenderJSONWithIDButUnknownStatus(t *testing.T) {
	r := &subagentRenderer{}
	theme := sdk.ThemeInfo{Foreground: "15"}

	// JSON with "id" but non-background status — should render as foreground.
	content := `{"id": "file-1", "status": "ok", "content": "found 3 matches"}`
	result := r.Render(content, theme, 80)

	// Should NOT render as a background card.
	assert.NotContains(t, result, "↗")
	// Should fall through to foreground output rendering.
	assert.Contains(t, result, "file-1")
}

// Ensure the renderer implements the interface.
var _ tui.RichToolRenderer = (*subagentRenderer)(nil)
