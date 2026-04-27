package messages

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolPanel_NewPanel_Pending(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls -la")
	assert.Equal(t, "tc1", p.ToolID())
	assert.Equal(t, "tc1", p.ItemID())
	assert.Equal(t, ToolPending, p.State())
	assert.False(t, p.Expanded())
}

func TestToolPanel_SetResult_Success(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("file1.txt\nfile2.txt", false)
	assert.Equal(t, ToolSuccess, p.State())
}

func TestToolPanel_SetResult_Error(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("command not found", true)
	assert.Equal(t, ToolError, p.State())
}

func TestToolPanel_ToggleExpanded(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	assert.False(t, p.Expanded())
	p.ToggleExpanded()
	assert.True(t, p.Expanded())
	p.ToggleExpanded()
	assert.False(t, p.Expanded())
}

func TestToolPanel_View_Pending(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls -la")
	view := p.View(80)
	assert.Contains(t, view, "bash")
	assert.Contains(t, view, "running...")
}

func TestToolPanel_View_Success(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("file1.txt\nfile2.txt", false)
	view := p.View(80)
	assert.Contains(t, view, "bash")
	assert.Contains(t, view, "file1.txt")
	assert.Contains(t, view, "file2.txt")
}

func TestToolPanel_View_Error(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("permission denied", true)
	view := p.View(80)
	assert.Contains(t, view, "bash")
	assert.Contains(t, view, "permission denied")
}

func TestToolPanel_View_NoOutput(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("", false)
	view := p.View(80)
	assert.Contains(t, view, "no output")
}

func TestToolPanel_View_WithArgs(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls -la /tmp")
	view := p.View(80)
	assert.Contains(t, view, "bash")
	assert.Contains(t, view, "ls -la /tmp")
}

func TestToolPanel_View_NoArgs(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "")
	view := p.View(80)
	assert.Contains(t, view, "bash")
	assert.NotContains(t, view, "()")
}

func TestToolPanel_CollapseLongOutput(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line of output"
	}

	output := strings.Join(lines, "\n")

	p := NewToolPanel("tc1", "bash", "cat bigfile")
	p.SetResult(output, false)
	view := p.View(80)

	assert.Contains(t, view, "10 more lines (collapsed)")
	assert.False(t, p.Expanded())
}

func TestToolPanel_ExpandShowsAll(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line of output"
	}

	output := strings.Join(lines, "\n")

	p := NewToolPanel("tc1", "bash", "cat bigfile")
	p.SetResult(output, false)
	p.ToggleExpanded()
	view := p.View(80)

	assert.NotContains(t, view, "collapsed")
	assert.True(t, p.Expanded())
	// All 30 lines should be present
	assert.Equal(t, 30, strings.Count(view, "line of output"))
}

func TestToolPanel_ShortOutputNotCollapsed(t *testing.T) {
	output := "short output\njust two lines"
	p := NewToolPanel("tc1", "bash", "echo hi")
	p.SetResult(output, false)
	view := p.View(80)

	assert.NotContains(t, view, "collapsed")
}

func TestTruncateArgs(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty", "", ""},
		{"short", "ls -la", "ls -la"},
		{"newlines", "line1\nline2\nline3", "line1 line2 line3"},
		{"long", strings.Repeat("x", 150), strings.Repeat("x", 97) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateArgs(tt.input, 100)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestToolPanel_StateTransitions(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	require.Equal(t, ToolPending, p.State())

	// Pending -> success
	p.SetResult("ok", false)
	assert.Equal(t, ToolSuccess, p.State())

	// Success -> error (reused panel)
	p.SetResult("fail", true)
	assert.Equal(t, ToolError, p.State())
}

func TestToolPanel_Draw_Pending(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls -la")
	canvas := uv.NewScreenBuffer(80, 10)
	p.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "bash")
	assert.Contains(t, output, "running...")
}

func TestToolPanel_Draw_Success(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("file1.txt\nfile2.txt", false)

	canvas := uv.NewScreenBuffer(80, 10)
	p.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "file1.txt")
}

func TestToolPanel_Draw_Error(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	p.SetResult("permission denied", true)

	canvas := uv.NewScreenBuffer(80, 10)
	p.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "permission denied")
}

func TestToolPanel_Draw_ZeroArea(t *testing.T) {
	p := NewToolPanel("tc1", "bash", "ls")
	canvas := uv.NewScreenBuffer(80, 10)
	p.Draw(canvas, uv.Rect(0, 0, 0, 0))
}
