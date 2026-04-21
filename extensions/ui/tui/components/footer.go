package components

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FooterModel renders a two-line status bar with context information.
type FooterModel struct {
	width int

	// Line 1: CWD + git branch
	cwd        string
	gitBranch  string
	branchDirty bool

	// Line 2: tokens + cost + context % + model + provider
	inputTokens  int
	outputTokens int
	cost         float64
	contextPct   float64 // 0-100
	modelName    string
	providerName string

	// Extension status entries (set by cross-extension UI)
	extStatus map[string]string
}

// NewFooterModel creates a new footer model.
func NewFooterModel() FooterModel {
	cwd, _ := os.Getwd()
	f := FooterModel{
		width:      80,
		cwd:        cwd,
		extStatus:  make(map[string]string),
	}
	f.gitBranch, f.branchDirty = getGitBranch()
	return f
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

// SetExtStatus sets an extension status entry.
func (m FooterModel) SetExtStatus(key, text string) FooterModel {
	m.extStatus[key] = text
	return m
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
	line2 := m.renderLine2()
	return line1 + "\n" + line2
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

	// Extension status entries
	for _, v := range m.extStatus {
		parts = append(parts, v)
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return dimStyle.Render(strings.Join(parts, " │ "))
}

func (m FooterModel) renderLine2() string {
	parts := []string{}

	// Token counts
	if m.inputTokens > 0 || m.outputTokens > 0 {
		parts = append(parts, fmt.Sprintf("in:%d out:%d", m.inputTokens, m.outputTokens))
	}

	// Cost
	if m.cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", m.cost))
	}

	// Context percentage with color thresholds
	if m.contextPct > 0 {
		pctStyle := lipgloss.NewStyle()
		switch {
		case m.contextPct > 90:
			pctStyle = pctStyle.Foreground(lipgloss.Color("196")) // red
		case m.contextPct > 70:
			pctStyle = pctStyle.Foreground(lipgloss.Color("220")) // yellow
		default:
			pctStyle = pctStyle.Foreground(lipgloss.Color("82")) // green
		}
		parts = append(parts, pctStyle.Render(fmt.Sprintf("ctx:%.0f%%", m.contextPct)))
	}

	// Model + provider
	if m.modelName != "" {
		modelDisplay := m.modelName
		if m.providerName != "" {
			modelDisplay = m.providerName + "/" + m.modelName
		}
		parts = append(parts, modelDisplay)
	}

	if len(parts) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		return dimStyle.Render("weave")
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return dimStyle.Render(strings.Join(parts, " │ "))
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string, maxWidth int) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}

	if maxWidth > 0 && len(path) > maxWidth {
		path = "..." + path[len(path)-maxWidth+3:]
	}
	return path
}

// getGitBranch returns the current git branch and dirty state.
func getGitBranch() (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))

	// Check dirty state
	cmd2 := exec.Command("git", "status", "--porcelain")
	out2, err := cmd2.Output()
	dirty := err == nil && len(strings.TrimSpace(string(out2))) > 0

	return branch, dirty
}
