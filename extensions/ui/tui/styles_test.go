package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave-tui/components"
	"github.com/weave-agent/weave-tui/palette"
)

// TestLipglossV2_NewStyleRendering verifies that basic lipgloss v2 style
// creation and rendering produces non-empty output.
func TestLipglossV2_NewStyleRendering(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Accent)).Bold(true)
	rendered := style.Render("hello")
	assert.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "hello")
}

// TestLipglossV2_StyleChaining verifies that method chaining on styles
// produces expected results (foreground + background + bold).
func TestLipglossV2_StyleChaining(t *testing.T) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.DefaultTheme().Foreground)).
		Background(lipgloss.Color(palette.DefaultTheme().Accent)).
		Bold(true)

	rendered := style.Render("test")
	assert.Contains(t, rendered, "test")
}

// TestLipglossV2_FaintStyle verifies Faint (dimmed) rendering.
func TestLipglossV2_FaintStyle(t *testing.T) {
	style := lipgloss.NewStyle().Faint(true)
	rendered := style.Render("dimmed")
	assert.Contains(t, rendered, "dimmed")
}

// TestLipglossV2_WidthConstraint verifies that Width() constrains rendered output.
func TestLipglossV2_WidthConstraint(t *testing.T) {
	style := lipgloss.NewStyle().Width(20)
	rendered := style.Render("hi")
	assert.Contains(t, rendered, "hi")
}

// TestLipglossV2_BorderRendering verifies Border + BorderForeground rendering.
func TestLipglossV2_BorderRendering(t *testing.T) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(palette.DefaultTheme().Accent)).
		Width(20).
		Padding(0, 1)

	rendered := style.Render("content")
	assert.Contains(t, rendered, "content")
	// Border should add visual characters
	assert.Contains(t, rendered, "╭")
}

// TestLipglossV2_NormalBorderRendering verifies NormalBorder.
func TestLipglossV2_NormalBorderRendering(t *testing.T) {
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(20)

	rendered := style.Render("test")
	assert.Contains(t, rendered, "test")
}

// TestLipglossV2_PaddingRendering verifies Padding adds space around content.
func TestLipglossV2_PaddingRendering(t *testing.T) {
	noPad := lipgloss.NewStyle().Render("x")
	withPad := lipgloss.NewStyle().Padding(0, 2).Render("x")
	assert.NotEqual(t, noPad, withPad, "padding should change output")
	assert.Contains(t, withPad, "x")
}

// TestLipglossV2_MarginRendering verifies Margin offsets content.
func TestLipglossV2_MarginRendering(t *testing.T) {
	style := lipgloss.NewStyle().MarginTop(1)
	rendered := style.Render("offset")
	assert.Contains(t, rendered, "offset")
}

// TestLipglossV2_JoinVertical verifies JoinVertical merges strings.
func TestLipglossV2_JoinVertical(t *testing.T) {
	result := lipgloss.JoinVertical(lipgloss.Left, "line1", "line2")
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[0], "line1")
	assert.Contains(t, lines[1], "line2")
}

// TestLipglossV2_JoinHorizontal verifies JoinHorizontal merges strings.
func TestLipglossV2_JoinHorizontal(t *testing.T) {
	result := lipgloss.JoinHorizontal(lipgloss.Top, "left", "right")
	assert.Contains(t, result, "left")
	assert.Contains(t, result, "right")
}

// TestLipglossV2_WidthMeasurement verifies lipgloss.Width measures rendered strings.
func TestLipglossV2_WidthMeasurement(t *testing.T) {
	plain := "hello"
	w := lipgloss.Width(plain)
	assert.Equal(t, 5, w)

	styled := lipgloss.NewStyle().Bold(true).Render("hello")
	w2 := lipgloss.Width(styled)
	assert.Equal(t, 5, w2, "Width should measure visible chars, not ANSI codes")
}

// TestLipglossV2_ColorFunction verifies lipgloss.Color creates valid color values.
func TestLipglossV2_ColorFunction(t *testing.T) {
	c := lipgloss.Color(palette.DefaultTheme().Accent)
	assert.NotNil(t, c)

	style := lipgloss.NewStyle().Foreground(c)
	rendered := style.Render("colored")
	assert.Contains(t, rendered, "colored")
}

// TestLipglossV2_RoundedBorder verifies RoundedBorder returns a valid border.
func TestLipglossV2_RoundedBorder(t *testing.T) {
	b := lipgloss.RoundedBorder()
	assert.NotNil(t, b)
}

// TestLipglossV2_StyleComposition verifies that two styles can be composed
// via inline rendering (inner content styled differently from outer).
func TestLipglossV2_StyleComposition(t *testing.T) {
	outer := lipgloss.NewStyle().Width(30)
	inner := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Success))

	content := inner.Render("green")
	rendered := outer.Render(content)
	assert.Contains(t, rendered, "green")
}

// TestScreenBuffer_StyleRendering verifies that styled strings render
// correctly through ultraviolet screen buffers (the TUI rendering path).
func TestScreenBuffer_StyleRendering(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Accent)).Bold(true)
	styled := style.Render("styled text")

	canvas := uv.NewScreenBuffer(40, 1)
	uv.NewStyledString(styled).Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())

	assert.Contains(t, output, "styled text")
}

// TestScreenBuffer_MultiStyleRendering verifies multiple styled segments
// in one line render correctly through screen buffers.
func TestScreenBuffer_MultiStyleRendering(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Error)).Render("red")
	green := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Success)).Render("green")
	combined := red + " " + green

	canvas := uv.NewScreenBuffer(40, 1)
	uv.NewStyledString(combined).Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())

	assert.Contains(t, output, "red")
	assert.Contains(t, output, "green")
}

// TestSpinnerV2_FunctionalOptions verifies that spinner.New with v2
// functional options creates a working spinner model.
func TestSpinnerV2_FunctionalOptions(t *testing.T) {
	s := components.NewSpinnerModel(palette.DefaultTheme())
	require.False(t, s.Visible())

	s = s.Show()
	require.True(t, s.Visible())

	view := s.View()
	assert.Contains(t, view, "Thinking...")
}
