package components

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// FooterModel renders a two-line status bar with context information.
type FooterModel struct {
	width int

	// Line 1: CWD + git branch
	cwd         string
	gitBranch   string
	branchDirty bool

	// Line 2: tokens + cost + context % + model + provider + thinking
	inputTokens   int
	outputTokens  int
	cost          float64
	contextPct    float64 // 0-100
	modelName     string
	providerName  string
	thinkingLevel string
	reasoning     bool

	// Token rate (placeholder for Phase 2 streaming enhancements)
	tokenRate float64

	// Cache tokens (prompt caching)
	cacheCreationTokens int
	cacheReadTokens     int

	// Extension status entries (set by cross-extension UI)
	extStatus map[string]string
}

// NewFooterModel creates a new footer model.
func NewFooterModel() FooterModel {
	cwd, _ := os.Getwd()
	f := FooterModel{
		width:     80,
		cwd:       cwd,
		extStatus: make(map[string]string),
	}
	f.gitBranch, f.branchDirty = getGitBranch()

	return f
}

// SetCWD updates the working directory display.
func (m FooterModel) SetCWD(cwd string) FooterModel {
	m.cwd = cwd
	return m
}

// SetSize updates the footer width.
func (m FooterModel) SetSize(width int) FooterModel {
	m.width = width
	return m
}

// Width returns the footer width.
func (m FooterModel) Width() int { return m.width }

// SetGitBranch updates the git branch display.
func (m FooterModel) SetGitBranch(branch string, dirty bool) FooterModel {
	m.gitBranch = branch
	m.branchDirty = dirty

	return m
}

// SetTokenUsage updates token counts and cost.
func (m FooterModel) SetTokenUsage(input, output int, cost float64) FooterModel {
	m.inputTokens = input
	m.outputTokens = output
	m.cost = cost

	return m
}

// SetCacheTokens updates cache token counts for prompt caching display.
func (m FooterModel) SetCacheTokens(creation, read int) FooterModel {
	m.cacheCreationTokens = creation
	m.cacheReadTokens = read

	return m
}

// SetContextPct updates the context window percentage (0-100).
func (m FooterModel) SetContextPct(pct float64) FooterModel {
	m.contextPct = pct
	return m
}

// SetModel updates the model and provider display.
func (m FooterModel) SetModel(model, provider string) FooterModel {
	m.modelName = model
	m.providerName = provider

	return m
}

// SetReasoning updates whether the current model supports reasoning.
func (m FooterModel) SetReasoning(reasoning bool) FooterModel {
	m.reasoning = reasoning
	return m
}

// SetThinkingLevel updates the thinking level display.
func (m FooterModel) SetThinkingLevel(level string) FooterModel {
	m.thinkingLevel = level
	return m
}

// ThinkingLevel returns the current thinking level.
func (m FooterModel) ThinkingLevel() string { return m.thinkingLevel }

// SetTokenRate updates the token rate display (placeholder for Phase 2).
func (m FooterModel) SetTokenRate(rate float64) FooterModel {
	m.tokenRate = rate
	return m
}

// TokenRate returns the current token rate.
func (m FooterModel) TokenRate() float64 { return m.tokenRate }

// SetExtStatus sets an extension status entry.
func (m FooterModel) SetExtStatus(key, text string) FooterModel {
	m.extStatus[key] = text
	return m
}

// ExtStatus returns the extension status map.
func (m FooterModel) ExtStatus() map[string]string {
	return m.extStatus
}

// InputTokens returns the input token count.
func (m FooterModel) InputTokens() int { return m.inputTokens }

// OutputTokens returns the output token count.
func (m FooterModel) OutputTokens() int { return m.outputTokens }

// Cost returns the current cost.
func (m FooterModel) Cost() float64 { return m.cost }

// ContextPct returns the context percentage.
func (m FooterModel) ContextPct() float64 { return m.contextPct }

// ModelName returns the model name.
func (m FooterModel) ModelName() string { return m.modelName }

// ProviderName returns the provider name.
func (m FooterModel) ProviderName() string { return m.providerName }

// GitBranch returns the current git branch.
func (m FooterModel) GitBranch() string { return m.gitBranch }

// View renders the two-line footer.
func (m FooterModel) View() string {
	if m.width <= 0 {
		return ""
	}

	line1 := m.renderLine1()
	line2 := m.renderLine2(nil)

	return line1 + "\n" + line2
}

// Draw renders the footer into a screen buffer region.
// Line 1 (CWD + git) goes into the first row, line 2 (tokens + model) into the second.
func (m FooterModel) Draw(scr uv.Screen, area uv.Rectangle, theme *palette.Theme) {
	if area.Dx() <= 0 || area.Dy() <= 0 || m.width <= 0 {
		return
	}

	if area.Dy() >= 1 {
		line1Rect := uv.Rect(area.Min.X, area.Min.Y, area.Dx(), 1)
		uv.NewStyledString(m.renderLine1()).Draw(scr, line1Rect)
	}

	if area.Dy() >= 2 {
		line2Rect := uv.Rect(area.Min.X, area.Min.Y+1, area.Dx(), 1)
		uv.NewStyledString(m.renderLine2(theme)).Draw(scr, line2Rect)
	}
}

func (m FooterModel) renderLine1() string {
	cwd := shortenPath(m.cwd, m.width/2)

	parts := []string{cwd}

	if m.gitBranch != "" {
		branch := m.gitBranch
		if m.branchDirty {
			branch += "*"
		}

		parts = append(parts, branch)
	}

	// Extension status entries (sorted for deterministic render order)
	keys := make([]string, 0, len(m.extStatus))

	for k := range m.extStatus {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, m.extStatus[k])
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Muted))

	return dimStyle.Render(strings.Join(parts, " · "))
}

func (m FooterModel) renderLine2(theme *palette.Theme) string {
	if theme == nil {
		theme = palette.DefaultTheme()
	}

	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))

	// Left side: stats (tokens, cost, context)
	leftParts := []string{}

	if m.inputTokens > 0 || m.outputTokens > 0 {
		leftParts = append(leftParts, mutedStyle.Render(fmt.Sprintf("in:%d out:%d", m.inputTokens, m.outputTokens)))
	}

	if m.cacheCreationTokens > 0 || m.cacheReadTokens > 0 {
		leftParts = append(leftParts, mutedStyle.Render(fmt.Sprintf("cache:+%d ~%d", m.cacheCreationTokens, m.cacheReadTokens)))
	}

	if m.cost > 0 {
		leftParts = append(leftParts, mutedStyle.Render(fmt.Sprintf("$%.4f", m.cost)))
	}

	if m.contextPct > 0 {
		pctStyle := lipgloss.NewStyle()

		switch {
		case m.contextPct > 90:
			pctStyle = pctStyle.Foreground(lipgloss.Color(theme.Error))
		case m.contextPct > 70:
			pctStyle = pctStyle.Foreground(lipgloss.Color(theme.Warning))
		default:
			pctStyle = pctStyle.Foreground(lipgloss.Color(theme.Success))
		}

		leftParts = append(leftParts, pctStyle.Render(fmt.Sprintf("ctx:%.0f%%", m.contextPct)))
	}

	left := strings.Join(leftParts, " ")

	// Right side: model info (model name, thinking level, token rate)
	rightParts := []string{}

	if m.modelName != "" {
		modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)).Bold(true)
		modelDisplay := m.modelName

		if m.providerName != "" {
			modelDisplay = m.providerName + "/" + m.modelName
		}

		rightParts = append(rightParts, modelStyle.Render(modelDisplay))
	}

	if m.thinkingLevel != "" && m.reasoning {
		pillStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Background(lipgloss.Color(theme.BackgroundTint)).
			Padding(0, 1)
		rightParts = append(rightParts, pillStyle.Render(m.thinkingLevel))
	}

	if m.tokenRate > 0 {
		rightParts = append(rightParts, mutedStyle.Render(fmt.Sprintf("%.1f tok/s", m.tokenRate)))
	}

	right := strings.Join(rightParts, " ")

	if left == "" && right == "" {
		return mutedStyle.Render("weave")
	}

	if left == "" {
		return right
	}

	if right == "" {
		return left
	}

	// Pad with spaces to push right group to the right edge
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	padding := max(m.width-leftWidth-rightWidth, 1)

	return left + strings.Repeat(" ", padding) + right
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string, maxWidth int) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}

	if maxWidth > 3 && utf8.RuneCountInString(path) > maxWidth {
		runes := []rune(path)
		path = "..." + string(runes[len(runes)-maxWidth+3:])
	}

	return path
}

// getGitBranch returns the current git branch and dirty state.
const gitTimeout = 500 * time.Millisecond

func getGitBranch() (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")

	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	branch := strings.TrimSpace(string(out))

	// Check dirty state
	ctx2, cancel2 := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, "git", "status", "--porcelain")
	out2, err2 := cmd2.Output()
	dirty := err2 == nil && strings.TrimSpace(string(out2)) != ""

	return branch, dirty
}
