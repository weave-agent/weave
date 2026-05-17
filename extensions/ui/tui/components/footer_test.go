package components

import (
	"os"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
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

func TestFooterModel_SetCacheTokens(t *testing.T) {
	f := NewFooterModel().SetCacheTokens(500, 2000)
	assert.Equal(t, 500, f.cacheCreationTokens)
	assert.Equal(t, 2000, f.cacheReadTokens)
}

func TestFooterModel_RenderLine2_WithCacheTokens(t *testing.T) {
	f := NewFooterModel().
		SetSize(120).
		SetTokenUsage(1000, 500, 0).
		SetCacheTokens(200, 800)
	line2 := f.renderLine2(nil)
	assert.Contains(t, line2, "in:1000 out:500")
	assert.Contains(t, line2, "cache:+200 ~800")
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

func TestFooterView_ContextPctAt70_IsSuccess(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(70)
	view := f.View()
	assert.Contains(t, view, "ctx:70%")
	// 70% should use Success color (114), not Warning (172)
	assert.Contains(t, view, "114")
}

func TestFooterView_ContextPctAt71_IsWarning(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(71)
	view := f.View()
	assert.Contains(t, view, "ctx:71%")
	// 71% should use Warning color (172), not Success (114)
	assert.Contains(t, view, "172")
}

func TestFooterView_ContextPctAt90_IsWarning(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(90)
	view := f.View()
	assert.Contains(t, view, "ctx:90%")
	// 90% should use Warning color (172), not Error (167)
	assert.Contains(t, view, "172")
}

func TestFooterView_ContextPctAt91_IsError(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetContextPct(91)
	view := f.View()
	assert.Contains(t, view, "ctx:91%")
	// 91% should use Error color (167)
	assert.Contains(t, view, "167")
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
	branch, dirty := getGitBranch()
	// In this project's test environment, we should always be in a git repo.
	assert.NotEmpty(t, branch, "getGitBranch should return a branch in a git repo")
	t.Logf("branch=%q dirty=%v", branch, dirty)
}

func TestFooterModel_SetThinkingLevel(t *testing.T) {
	f := NewFooterModel().SetThinkingLevel("medium")
	assert.Equal(t, "medium", f.ThinkingLevel())
}

func TestFooterView_ThinkingLevelAfterModel(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetReasoning(true).SetThinkingLevel("high")
	view := f.View()
	assert.Contains(t, view, "anthropic/claude-sonnet-4")
	assert.Contains(t, view, "high")
}

func TestFooterView_ThinkingLevelEmpty(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic")
	view := f.View()
	assert.Contains(t, view, "anthropic/claude-sonnet-4")
	// No thinking level text on line 2
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	assert.NotContains(t, lines[1], "medium")
}

func TestFooterView_ThinkingLevelOff(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("gpt-5.5", "openai").SetReasoning(true).SetThinkingLevel("off")
	view := f.View()
	assert.Contains(t, view, "openai/gpt-5.5")
	assert.Contains(t, view, "off")
}

func TestFooterView_ThinkingLevelHiddenForNonReasoning(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("gpt-5.5", "openai").SetReasoning(false).SetThinkingLevel("high")
	view := f.View()
	assert.Contains(t, view, "openai/gpt-5.5")
	assert.NotContains(t, view, " · high")
}

func TestFooterView_ThinkingLevelShownForReasoning(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetReasoning(true).SetThinkingLevel("medium")
	view := f.View()
	assert.Contains(t, view, "anthropic/claude-sonnet-4")
	assert.Contains(t, view, "medium")
}

func TestFooterModel_Draw(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic")
	canvas := uv.NewScreenBuffer(80, 2)
	f.Draw(canvas, canvas.Bounds(), nil)
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "anthropic/claude-sonnet-4")
}

func TestFooterModel_Draw_TwoLines(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetTokenUsage(100, 50, 0.01)
	canvas := uv.NewScreenBuffer(80, 2)
	f.Draw(canvas, canvas.Bounds(), nil)
	output := uv.TrimSpace(canvas.Render())
	lines := strings.Split(output, "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[1], "in:100")
	assert.Contains(t, lines[1], "out:50")
}

func TestFooterModel_Draw_ZeroArea(t *testing.T) {
	f := NewFooterModel().SetSize(80)
	canvas := uv.NewScreenBuffer(80, 2)
	f.Draw(canvas, uv.Rect(0, 0, 0, 0), nil)
}

func TestFooterModel_Draw_SingleRow(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetCWD("/home/user/weave")
	canvas := uv.NewScreenBuffer(80, 1)
	f.Draw(canvas, canvas.Bounds(), nil)
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "weave")
}

func TestFooterModel_SetTokenRate(t *testing.T) {
	f := NewFooterModel().SetTokenRate(42.5)
	assert.InDelta(t, 42.5, f.TokenRate(), 0.01)
}

func TestFooterView_TokenRateDisplayed(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetTokenRate(42.5)
	view := f.View()
	assert.Contains(t, view, "42.5 tok/s")
}

func TestFooterView_TokenRateClearedWhenZero(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetTokenRate(0)
	view := f.View()
	assert.NotContains(t, view, "tok/s")
}

func TestFooterView_TokenRateInDraw(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetTokenRate(123.4)
	canvas := uv.NewScreenBuffer(80, 2)
	f.Draw(canvas, canvas.Bounds(), nil)
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "123.4 tok/s")
}

// --- Task 4: Footer redesign tests ---

func TestFooterView_ModelNameBoldAndPrimary(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic")
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	// Model name should be on line 2 with ANSI bold + primary color codes
	assert.Contains(t, lines[1], "anthropic/claude-sonnet-4")
	// Bold ANSI code (1;) should be present in the styled output
	assert.Contains(t, lines[1], "\x1b[1;")
}

func TestFooterView_LeftRightGrouping(t *testing.T) {
	f := NewFooterModel().SetSize(80).
		SetTokenUsage(100, 50, 0.01).
		SetContextPct(50).
		SetModel("claude-sonnet-4", "anthropic")
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	line2 := lines[1]
	// Both stats and model should be present
	assert.Contains(t, line2, "in:100")
	assert.Contains(t, line2, "anthropic/claude-sonnet-4")
	// Model name should appear after stats with padding between
	idxIn := strings.Index(line2, "in:100")
	idxModel := strings.Index(line2, "anthropic/claude-sonnet-4")
	require.Greater(t, idxModel, idxIn, "model should appear after stats (right-aligned)")
}

func TestFooterView_ThinkingLevelAsPill(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic").SetReasoning(true).SetThinkingLevel("medium")
	view := f.View()
	// Thinking level should appear
	assert.Contains(t, view, "medium")
}

func TestFooterView_StatsMuted(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetTokenUsage(100, 50, 0)
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	// Token counts should appear with muted color
	assert.Contains(t, lines[1], "in:100")
	assert.Contains(t, lines[1], "out:50")
	// Verify muted color code is applied
	assert.Contains(t, lines[1], "38;5;240", "stats should render with muted color")
}

func TestFooterView_ModelAndStatsBothPresent(t *testing.T) {
	f := NewFooterModel().SetSize(80).
		SetTokenUsage(1000, 200, 0.005).
		SetContextPct(30).
		SetModel("gpt-5.5", "openai").
		SetTokenRate(25.5)
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	line2 := lines[1]
	// All left-side stats
	assert.Contains(t, line2, "in:1000")
	assert.Contains(t, line2, "out:200")
	assert.Contains(t, line2, "$0.0050")
	assert.Contains(t, line2, "ctx:30%")
	// All right-side model info
	assert.Contains(t, line2, "openai/gpt-5.5")
	assert.Contains(t, line2, "25.5 tok/s")
}

func TestFooterView_EmptyModelWithStats(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetTokenUsage(100, 50, 0)
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	// Only left side should be present, no model info
	assert.Contains(t, lines[1], "in:100")
	assert.NotContains(t, lines[1], "anthropic")
}

func TestFooterView_ModelOnlyNoStats(t *testing.T) {
	f := NewFooterModel().SetSize(80).SetModel("claude-sonnet-4", "anthropic")
	view := f.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	// Only right side should be present
	assert.Contains(t, lines[1], "anthropic/claude-sonnet-4")
	assert.NotContains(t, lines[1], "in:")
}
