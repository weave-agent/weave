package messages

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserMessage_Content(t *testing.T) {
	m := NewUserMessage("hello agent")
	assert.Equal(t, "hello agent", m.Content())
}

func TestUserMessage_View_PlainText(t *testing.T) {
	m := NewUserMessage("fix the bug")
	view := m.View(80)
	assert.Contains(t, view, "fix the bug")
	assert.Contains(t, view, "▐")
}

func TestUserMessage_EmptyContent(t *testing.T) {
	m := NewUserMessage("")
	assert.Empty(t, m.Content())
	// Empty message still renders bar styling
	view := m.View(80)
	assert.Contains(t, view, "▐")
}

func TestUserMessage_View_ZeroWidth(t *testing.T) {
	m := NewUserMessage("<skill name=\"test\">\nbody\n</skill>")
	view := m.View(0)
	assert.Contains(t, view, "[skill test]")
}

func TestParseSkillXML_Valid(t *testing.T) {
	content := "<skill name=\"my-skill\">\ninstructions here\n</skill>\n\ndo something"
	block, ok := parseSkillXML(content)
	assert.True(t, ok)
	assert.Equal(t, "my-skill", block.name)
	assert.Equal(t, "instructions here", block.body)
	assert.Equal(t, "do something", block.trailing)
}

func TestParseSkillXML_WithLocation(t *testing.T) {
	content := "<skill name=\"analyze\" location=\"/path/to/skill/SKILL.md\">\nbody\n</skill>"
	block, ok := parseSkillXML(content)
	assert.True(t, ok)
	assert.Equal(t, "analyze", block.name)
	assert.Equal(t, "body", block.body)
}

func TestParseSkillXML_NoTrailing(t *testing.T) {
	content := "<skill name=\"test\">\nbody\n</skill>"
	block, ok := parseSkillXML(content)
	assert.True(t, ok)
	assert.Equal(t, "test", block.name)
	assert.Empty(t, block.trailing)
}

func TestParseSkillXML_NoMatch(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"plain text", "just a regular message"},
		{"empty", ""},
		{"malformed xml", "<skill>no name attr</skill>"},
		{"unclosed", "<skill name=\"test\">\nbody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseSkillXML(tt.content)
			assert.False(t, ok)
		})
	}
}

func TestUserMessage_IsSkillInvocation(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"skill xml", "<skill name=\"test\">\nbody\n</skill>", true},
		{"plain text", "regular message", false},
		{"empty", "", false},
		{"malformed", "<skill>no name</skill>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewUserMessage(tt.content)
			assert.Equal(t, tt.expected, m.IsSkillInvocation())
		})
	}
}

func TestUserMessage_View_SkillCollapsed(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-skill\">\nfull instructions here\n</skill>\n\ndo something")
	view := m.View(80)
	assert.Contains(t, view, "[skill my-skill]")
	assert.Contains(t, view, "do something")
	assert.NotContains(t, view, "full instructions")
}

func TestUserMessage_View_SkillCollapsed_NoArgs(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-skill\">\nfull instructions\n</skill>")
	view := m.View(80)
	assert.Contains(t, view, "[skill my-skill]")
	assert.NotContains(t, view, "full instructions")
}

func TestUserMessage_View_SkillExpanded(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-skill\">\nfull instructions here\n</skill>\n\ndo something")
	m.ToggleExpanded()
	view := m.View(80)
	assert.Contains(t, view, "[skill my-skill]")
	assert.Contains(t, view, "full instructions here")
	assert.Contains(t, view, "do something")
}

func TestUserMessage_View_SkillExpanded_NoBody(t *testing.T) {
	m := NewUserMessage("<skill name=\"test\">\n</skill>\n\nargs here")
	m.ToggleExpanded()
	view := m.View(80)
	assert.Contains(t, view, "[skill test]")
	assert.Contains(t, view, "args here")
}

func TestUserMessage_ToggleExpanded(t *testing.T) {
	m := NewUserMessage("<skill name=\"test\">\nbody\n</skill>")
	assert.False(t, m.Expanded())
	m.ToggleExpanded()
	assert.True(t, m.Expanded())
	m.ToggleExpanded()
	assert.False(t, m.Expanded())
}

func TestUserMessage_View_SkillSpecialCharsInName(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-cool-skill\">\nbody\n</skill>\n\nargs")
	view := m.View(80)
	assert.Contains(t, view, "[skill my-cool-skill]")
}

func TestUserMessage_View_PlainTextNotAffected(t *testing.T) {
	m := NewUserMessage("regular message without xml")
	view := m.View(80)
	assert.Contains(t, view, "regular message without xml")
	assert.Contains(t, view, "▐")
}

func TestUserMessage_Draw_PlainText(t *testing.T) {
	m := NewUserMessage("hello world")
	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "hello world")
}

func TestUserMessage_Draw_SkillCollapsed(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-skill\">\nbody\n</skill>\n\ntrailing")
	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "[skill my-skill]")
	assert.NotContains(t, output, "body")
}

func TestUserMessage_Draw_SkillExpanded(t *testing.T) {
	m := NewUserMessage("<skill name=\"my-skill\">\nfull instructions\n</skill>")
	m.ToggleExpanded()

	canvas := uv.NewScreenBuffer(80, 10)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "full instructions")
}

func TestUserMessage_Draw_MultilineClips(t *testing.T) {
	m := NewUserMessage("line1\nline2\nline3\nline4\nline5")
	canvas := uv.NewScreenBuffer(80, 2)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	lines := strings.Split(output, "\n")
	assert.LessOrEqual(t, len(lines), 2)
}

func TestUserMessage_Draw_ZeroArea(t *testing.T) {
	m := NewUserMessage("hello")
	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

// --- Styled user message tests (Task 2) ---

func TestUserMessage_Styling_HasBorderAndPrefix(t *testing.T) {
	m := NewUserMessage("test message")
	view := m.View(80)

	assert.Contains(t, view, "▐", "should have left-edge bar")
}

func TestUserMessage_Styling_MultilineHasPrefixOnEachLine(t *testing.T) {
	m := NewUserMessage("line1\nline2\nline3")
	view := m.View(80)

	lines := strings.Split(view, "\n")
	require.Len(t, lines, 3)

	for _, line := range lines {
		assert.Contains(t, line, "▐", "each line should have left-edge bar")
	}
}

func TestUserMessage_Styling_SkillHasBorderAndPrefix(t *testing.T) {
	m := NewUserMessage("<skill name=\"test\">\nbody\n</skill>")
	view := m.View(80)

	assert.Contains(t, view, "▐")
	assert.Contains(t, view, "[skill test]")
}
