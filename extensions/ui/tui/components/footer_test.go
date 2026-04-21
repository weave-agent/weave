package components

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFooterModel(t *testing.T) {
	f := NewFooterModel()
	assert.Equal(t, 80, f.Width())
	assert.NotEmpty(t, f.cwd)
}

func TestFooterModel_SetSize(t *testing.T) {
	f := NewFooterModel().SetSize(120)
	assert.Equal(t, 120, f.Width())
}

func TestFooterModel_SetGitBranch(t *testing.T) {
	f := NewFooterModel().SetGitBranch("main", false)
	assert.Equal(t, "main", f.GitBranch())
}

func TestFooterModel_SetTokenUsage(t *testing.T) {
	f := NewFooterModel().SetTokenUsage(100, 50, 0.0123)
	assert.Equal(t, 100, f.InputTokens())
	assert.Equal(t, 50, f.OutputTokens())
	assert.InDelta(t, 0.0123, f.Cost(), 0.0001)
}

func TestFooterModel_SetContextPct(t *testing.T) {
	f := NewFooterModel().SetContextPct(42.5)
	assert.InDelta(t, 42.5, f.ContextPct(), 0.01)
}

func TestFooterModel_SetModel(t *testing.T) {
	f := NewFooterModel().SetModel("claude-sonnet-4", "anthropic")
	assert.Equal(t, "claude-sonnet-4", f.ModelName())
	assert.Equal(t, "anthropic", f.ProviderName())
}

func TestFooterModel_SetExtStatus(t *testing.T) {
	f := NewFooterModel().SetExtStatus("git", "main")
	assert.Equal(t, "main", f.extStatus["git"])
}

func TestFooterView_RendersTwoLines(t *testing.T) {
	f := NewFooterModel().SetSize(80)
	view := f.View()
	lines := strings.Split(view, "\n")
	assert.Len(t, lines, 2)
}

func TestFooterView_Line1ContainsCWD(t *testing.T) {
	f := NewFooterModel().SetSize(200)
	view := f.View()
	lines := strings.Split(view, "\n")
	// Line 1 should contain ~-substituted path
	assert.Contains(t, lines[0], "~/Projects/weave")
}

func TestFooterView_Line1ContainsGitBranch(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetGitBranch("feature-branch", false)
	view := f.View()
	assert.Contains(t, view, "feature-branch")
}

func TestFooterView_Line1DirtyBranch(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetGitBranch("main", true)
	view := f.View()
	assert.Contains(t, view, "main*")
}

func TestFooterView_Line2ContainsTokens(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetTokenUsage(1500, 300, 0)
	view := f.View()
	assert.Contains(t, view, "in:1500")
	assert.Contains(t, view, "out:300")
}

func TestFooterView_Line2ContainsCost(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetTokenUsage(100, 50, 0.0123)
	view := f.View()
	assert.Contains(t, view, "$0.0123")
}

func TestFooterView_Line2ContainsModel(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic")
	view := f.View()
	assert.Contains(t, view, "anthropic/claude-sonnet-4")
}

func TestFooterView_ContextPctGreen(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(50)
	view := f.View()
	assert.Contains(t, view, "ctx:50%")
}

func TestFooterView_ContextPctYellow(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(80)
	view := f.View()
	assert.Contains(t, view, "ctx:80%")
}

func TestFooterView_ContextPctRed(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(95)
	view := f.View()
	assert.Contains(t, view, "ctx:95%")
}

func TestFooterView_EmptyState(t *testing.T) {
	f := NewFooterModel().SetSize(80)
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[1], "weave")
}

func TestFooterView_ZeroWidth(t *testing.T) {
	f := NewFooterModel().SetSize(0)
	view := f.View()
	assert.Empty(t, view)
}

func TestShortenPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}

	tests := []struct {
		name     string
		path     string
		maxWidth int
		want     string
	}{
		{
			name:     "home substitution",
			path:     home + "/projects/myapp",
			maxWidth: 80,
			want:     "~/projects/myapp",
		},
		{
			name:     "path too long",
			path:     home + "/very/long/path/that/exceeds/max/width/characters",
			maxWidth: 20,
			want:     ".../width/characters",
		},
		{
			name:     "non-home path",
			path:     "/tmp/test",
			maxWidth: 80,
			want:     "/tmp/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenPath(tt.path, tt.maxWidth)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetGitBranch(t *testing.T) {
	// This test just ensures the function doesn't panic.
	// It may return empty if not in a git repo.
	branch, dirty := getGitBranch()
	t.Logf("branch=%q dirty=%v", branch, dirty)
	// In CI or non-git dirs, branch may be empty — that's fine
}
