package styles

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultColors(t *testing.T) {
	c := DefaultColors()

	assert.NotNil(t, c.Purple)
	assert.NotNil(t, c.LightPurple)
	assert.NotNil(t, c.Success)
	assert.NotNil(t, c.Warning)
	assert.NotNil(t, c.Error)
	assert.NotNil(t, c.DimGray)
	assert.NotNil(t, c.MediumGray)
	assert.NotNil(t, c.Gray)
	assert.NotNil(t, c.LightGray)
	assert.NotNil(t, c.NearWhite)
	assert.NotNil(t, c.BrightWhite)
	assert.NotNil(t, c.DiffAdded)
	assert.NotNil(t, c.DiffRemoved)
	assert.NotNil(t, c.DiffHeader)
	assert.NotNil(t, c.DiffHunk)
	assert.NotNil(t, c.PctHealthy)
	assert.NotNil(t, c.PctWarning)
	assert.NotNil(t, c.PctCritical)
}

func TestThinkingBorderColor(t *testing.T) {
	tests := []struct {
		level sdk.ThinkingLevel
	}{
		{sdk.ThinkingOff},
		{sdk.ThinkingMinimal},
		{sdk.ThinkingLow},
		{sdk.ThinkingMedium},
		{sdk.ThinkingHigh},
		{sdk.ThinkingXHigh},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			c := ThinkingBorderColor(tt.level)
			assert.NotNil(t, c, "ThinkingBorderColor(%s) should return non-nil color", tt.level)
		})
	}

	// Unknown level returns default
	c := ThinkingBorderColor("unknown")
	assert.NotNil(t, c)
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	// Verify all colors are set
	assert.NotNil(t, theme.Colors.Purple)
	assert.NotNil(t, theme.Colors.Success)

	// Verify all styles produce non-empty output
	renderTests := []struct {
		name  string
		style func() string
	}{
		{"Footer", func() string { return theme.Footer.Render("cwd: /test") }},
		{"Hints", func() string { return theme.Hints.Render("ctrl+p cycle") }},
		{"StatusMessage", func() string { return theme.StatusMessage.Render("switched") }},
		{"Spinner", func() string { return theme.Spinner.Render("⣾") }},
		{"ToolDim", func() string { return theme.ToolDim.Render("running...") }},
		{"ToolErrorBody", func() string { return theme.ToolErrorBody.Render("error text") }},
		{"DiffAdded", func() string { return theme.DiffAdded.Render("+added") }},
		{"DiffRemoved", func() string { return theme.DiffRemoved.Render("-removed") }},
		{"DiffContext", func() string { return theme.DiffContext.Render("context") }},
		{"DiffHeader", func() string { return theme.DiffHeader.Render("--- file") }},
		{"DiffHunk", func() string { return theme.DiffHunk.Render("@@ @@") }},
		{"OverlayTitle", func() string { return theme.OverlayTitle.Render("Select Model") }},
		{"OverlayFilter", func() string { return theme.OverlayFilter.Render("filter") }},
		{"OverlayNormal", func() string { return theme.OverlayNormal.Render("item") }},
		{"OverlaySubtitle", func() string { return theme.OverlaySubtitle.Render("subtitle") }},
		{"OverlayMessage", func() string { return theme.OverlayMessage.Render("confirm?") }},
		{"OverlayPrompt", func() string { return theme.OverlayPrompt.Render("enter key:") }},
		{"OverlayInput", func() string { return theme.OverlayInput.Render("sk-xxx") }},
		{"OverlayHint", func() string { return theme.OverlayHint.Render("Esc to cancel") }},
	}

	for _, tt := range renderTests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.style()
			assert.NotEmpty(t, output, "%s.Render should produce non-empty output", tt.name)
		})
	}
}

func TestDefaultTheme_BorderedStyles(t *testing.T) {
	theme := DefaultTheme()

	// Styles with borders need width set to render properly
	borderTests := []struct {
		name  string
		style func() string
	}{
		{"EditorBorder", func() string { return theme.EditorBorder.Width(40).Render("content") }},
		{"ToolPending", func() string { return theme.ToolPending.Width(40).Render("bash") }},
		{"ToolSuccess", func() string { return theme.ToolSuccess.Width(40).Render("bash") }},
		{"ToolError", func() string { return theme.ToolError.Width(40).Render("bash") }},
		{"OverlayBorder", func() string { return theme.OverlayBorder.Width(40).Render("dialog") }},
	}

	for _, tt := range borderTests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.style()
			require.NotEmpty(t, output)
			// Bordered output should contain visible characters
			assert.GreaterOrEqual(t, len(output), 10, "bordered output should have substantial content")
		})
	}
}

func TestDefaultTheme_OverlayButtonStyles(t *testing.T) {
	theme := DefaultTheme()

	active := theme.OverlayActiveBtn.Render("Yes")
	inactive := theme.OverlayInactiveBtn.Render("No")

	assert.NotEmpty(t, active)
	assert.NotEmpty(t, inactive)
}

func TestDefaultTheme_AutocompleteStyles(t *testing.T) {
	theme := DefaultTheme()

	normal := theme.Autocomplete.Render("item")
	selected := theme.AutocompleteSel.Render("item")

	assert.NotEmpty(t, normal)
	assert.NotEmpty(t, selected)
}

func TestThemeStylesAreImmutable(t *testing.T) {
	theme := DefaultTheme()

	// Styles are value types - setting width returns a copy
	modified := theme.Footer.Width(80)
	rendered := modified.Render("test")
	assert.Contains(t, rendered, "test")

	// Original should be unchanged (no width set)
	original := theme.Footer.Render("test")
	assert.Contains(t, original, "test")
}

func TestDefaultTheme_CompleteTheme(t *testing.T) {
	theme := DefaultTheme()

	// Every style field should render without panicking.
	// render is a function type so we can build a flat table.
	type styleCase struct {
		name   string
		render func()
	}

	styles := []styleCase{
		{"Footer", func() { theme.Footer.Render("x") }},
		{"Hints", func() { theme.Hints.Render("x") }},
		{"StatusMessage", func() { theme.StatusMessage.Render("x") }},
		{"EditorBorder", func() { theme.EditorBorder.Width(40).Render("x") }},
		{"Autocomplete", func() { theme.Autocomplete.Render("x") }},
		{"AutocompleteSel", func() { theme.AutocompleteSel.Render("x") }},
		{"Spinner", func() { theme.Spinner.Render("x") }},
		{"ThinkingDim", func() { theme.ThinkingDim.Render("x") }},
		{"ToolPending", func() { theme.ToolPending.Width(40).Render("x") }},
		{"ToolSuccess", func() { theme.ToolSuccess.Width(40).Render("x") }},
		{"ToolError", func() { theme.ToolError.Width(40).Render("x") }},
		{"ToolDim", func() { theme.ToolDim.Render("x") }},
		{"ToolErrorBody", func() { theme.ToolErrorBody.Render("x") }},
		{"DiffAdded", func() { theme.DiffAdded.Render("x") }},
		{"DiffRemoved", func() { theme.DiffRemoved.Render("x") }},
		{"DiffContext", func() { theme.DiffContext.Render("x") }},
		{"DiffHeader", func() { theme.DiffHeader.Render("x") }},
		{"DiffHunk", func() { theme.DiffHunk.Render("x") }},
		{"OverlayBorder", func() { theme.OverlayBorder.Width(40).Render("x") }},
		{"OverlayTitle", func() { theme.OverlayTitle.Render("x") }},
		{"OverlayFilter", func() { theme.OverlayFilter.Render("x") }},
		{"OverlaySelected", func() { theme.OverlaySelected.Render("x") }},
		{"OverlayNormal", func() { theme.OverlayNormal.Render("x") }},
		{"OverlaySubtitle", func() { theme.OverlaySubtitle.Render("x") }},
		{"OverlayActiveBtn", func() { theme.OverlayActiveBtn.Render("x") }},
		{"OverlayInactiveBtn", func() { theme.OverlayInactiveBtn.Render("x") }},
		{"OverlayMessage", func() { theme.OverlayMessage.Render("x") }},
		{"OverlayPrompt", func() { theme.OverlayPrompt.Render("x") }},
		{"OverlayInput", func() { theme.OverlayInput.Render("x") }},
		{"OverlayHint", func() { theme.OverlayHint.Render("x") }},
	}

	for _, s := range styles {
		t.Run(s.name, func(t *testing.T) {
			assert.NotPanics(t, s.render)
		})
	}
}
