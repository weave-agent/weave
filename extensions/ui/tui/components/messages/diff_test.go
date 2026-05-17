package messages

import (
	"strings"
	"testing"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleDiff = `--- a/main.go
+++ b/main.go
@@ -10,7 +10,7 @@
 import "fmt"
-	oldLine := "removed"
+	newLine := "added"
 	context := "unchanged"
-	another := "gone"
+	replacement := "new"
+	extra := "more"
`

func TestParseDiff_ValidUnified(t *testing.T) {
	lines := ParseDiff(sampleDiff)
	require.NotNil(t, lines)
	require.Len(t, lines, 11)

	assert.Equal(t, DiffHeader, lines[0].Kind)
	assert.Contains(t, lines[0].Content, "---")

	assert.Equal(t, DiffHeader, lines[1].Kind)
	assert.Contains(t, lines[1].Content, "+++")

	assert.Equal(t, DiffHunk, lines[2].Kind)
	assert.Contains(t, lines[2].Content, "@@")

	assert.Equal(t, DiffContext, lines[3].Kind)
	assert.Contains(t, lines[3].Content, "import")

	assert.Equal(t, DiffRemoved, lines[4].Kind)
	assert.Contains(t, lines[4].Content, "oldLine")

	assert.Equal(t, DiffAdded, lines[5].Kind)
	assert.Contains(t, lines[5].Content, "newLine")
}

func TestParseDiff_NotDiff(t *testing.T) {
	lines := ParseDiff("just some regular text\nwith multiple lines")
	assert.Nil(t, lines)
}

func TestParseDiff_PartialHeader(t *testing.T) {
	// Only --- without +++ is not a valid diff
	lines := ParseDiff("--- only header\nsome content\nmore content")
	assert.Nil(t, lines)
}

func TestIsDiffContent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"valid diff", sampleDiff, true},
		{"plain text", "hello world", false},
		{"only minus prefix", "- removed line\n+ added line", false},
		{"header with leading whitespace", "  --- a/file\n  +++ b/file\n  @@ -1 +1 @@", true},
		{"header without hunk marker", "  --- a/file\n  +++ b/file", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsDiffContent(tt.input))
		})
	}
}

func TestDiffRenderer_Render(t *testing.T) {
	r := NewDiffRenderer()
	output := r.Render(sampleDiff, 80)

	// Output should contain the text content
	assert.Contains(t, output, "--- a/main.go")
	assert.Contains(t, output, "+++ b/main.go")
	assert.Contains(t, output, "oldLine")
	assert.Contains(t, output, "newLine")
}

func TestDiffRenderer_Render_NonDiff(t *testing.T) {
	r := NewDiffRenderer()
	text := "just regular text, nothing to see"
	output := r.Render(text, 80)
	assert.Equal(t, text, output)
}

func TestDiffStats(t *testing.T) {
	added, removed := DiffStats(sampleDiff)
	assert.Equal(t, 3, added)   // newLine, replacement, extra
	assert.Equal(t, 2, removed) // oldLine, another
}

func TestDiffStats_NonDiff(t *testing.T) {
	added, removed := DiffStats("not a diff")
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, removed)
}

func TestFormatDiffStats(t *testing.T) {
	tests := []struct {
		name    string
		added   int
		removed int
		expect  string
	}{
		{"both", 5, 3, "+5/-3"},
		{"only added", 2, 0, "+2/-0"},
		{"only removed", 0, 4, "+0/-4"},
		{"none", 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, FormatDiffStats(tt.added, tt.removed))
		})
	}
}

func TestClassifyLine(t *testing.T) {
	tests := []struct {
		input string
		kind  DiffLineKind
	}{
		{"--- a/file", DiffHeader},
		{"+++ b/file", DiffHeader},
		{"@@ -1,3 +1,4 @@", DiffHunk},
		{"+added line", DiffAdded},
		{"-removed line", DiffRemoved},
		{" context line", DiffContext},
		{"", DiffContext},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			line := classifyLine(tt.input)
			assert.Equal(t, tt.kind, line.Kind)
			assert.Equal(t, tt.input, line.Content)
		})
	}
}

func TestDiffRenderer_AllLineTypes(t *testing.T) {
	input := `--- a/old.txt
+++ b/new.txt
@@ -1,3 +1,3 @@
 context line
-removed line
+added line
`
	r := NewDiffRenderer()
	output := r.Render(input, 80)

	// Verify all content is present
	assert.Contains(t, output, "--- a/old.txt")
	assert.Contains(t, output, "+++ b/new.txt")
	assert.Contains(t, output, "@@")
	assert.Contains(t, output, "context line")
	assert.Contains(t, output, "removed line")
	assert.Contains(t, output, "added line")
}

func TestDiffRenderer_LargeDiff(t *testing.T) {
	var bldr strings.Builder
	bldr.WriteString("--- a/file.go\n+++ b/file.go\n@@ -1,4 +1,4 @@\n")

	for range 100 {
		bldr.WriteString("-old line\n")
		bldr.WriteString("+new line\n")
	}

	bldr.WriteString(" context\n")

	r := NewDiffRenderer()
	output := r.Render(bldr.String(), 80)
	assert.Contains(t, output, "old line")
	assert.Contains(t, output, "new line")
}

func TestDiffRenderer_UsesThemeColors(t *testing.T) {
	r := NewDiffRenderer()
	theme := palette.DefaultTheme()

	// Verify the renderer uses theme-aligned colors.
	assert.Equal(t, lipgloss.Color(theme.Success), r.addedStyle.GetForeground())
	assert.Equal(t, lipgloss.Color(theme.Error), r.removedStyle.GetForeground())
	assert.Equal(t, lipgloss.Color(theme.Muted), r.contextStyle.GetForeground())
	assert.Equal(t, lipgloss.Color(theme.Accent), r.headerStyle.GetForeground())
	assert.Equal(t, lipgloss.Color(theme.AccentBright), r.hunkStyle.GetForeground())
}
