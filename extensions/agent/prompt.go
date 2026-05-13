package agent

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"weave/sdk"
)

//go:embed default-system-prompt.md
var defaultSystemPrompt string

type buildInput struct {
	contextFiles []contextFile
	systemBase   string
	systemAppend string
	skills       []Skill
}

// promptBuilder assembles the system prompt from multiple layers:
// default prompt or SYSTEM.md, date/CWD, tool descriptions, skills, context files, APPEND_SYSTEM.md.
type promptBuilder struct {
	cfg sdk.Config
}

func newPromptBuilder(cfg sdk.Config) *promptBuilder {
	return &promptBuilder{cfg: cfg}
}

// Build assembles the full system prompt from all layers.
// Layers (top to bottom):
//  1. Default prompt OR SYSTEM.md (if found)
//  2. Date + CWD (always injected)
//  3. Available tools (dynamic)
//  4. Skills XML + usage instructions
//  5. Context files (CLAUDE.md/AGENTS.md)
//  6. APPEND_SYSTEM.md
func (pb *promptBuilder) Build(input buildInput) string {
	var b strings.Builder

	// Layer 1: system base (default or SYSTEM.md)
	base := input.systemBase
	if base == "" {
		base = defaultSystemPrompt
	}

	b.WriteString(strings.TrimSpace(base))
	b.WriteString("\n\n")

	// Layer 2: date + CWD
	b.WriteString(pb.buildInjectedSection())
	b.WriteString("\n\n")

	// Layer 3: available tools
	toolsSection := pb.buildToolDescriptions()
	if toolsSection != "" {
		b.WriteString(toolsSection)
		b.WriteString("\n\n")
	}

	// Layer 4: skills XML + usage instructions
	skillsSection := formatSkillsPrompt(input.skills)
	if skillsSection != "" {
		b.WriteString(skillsSection)
		b.WriteString("\n\n")
		b.WriteString(pb.buildSkillsUsage())
		b.WriteString("\n\n")
	}

	// Layer 5: context files
	contextSection := pb.buildContextSection(input.contextFiles)
	if contextSection != "" {
		b.WriteString(contextSection)
		b.WriteString("\n\n")
	}

	// Layer 6: APPEND_SYSTEM.md
	if input.systemAppend != "" {
		b.WriteString(strings.TrimSpace(input.systemAppend))
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

// buildInjectedSection formats the always-injected date and CWD.
func (pb *promptBuilder) buildInjectedSection() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Current date: %s\n", time.Now().Format("2006-01-02"))

	cwd := ""
	if pb.cfg != nil {
		cwd = pb.cfg.ProjectDir()
	}

	if cwd == "" {
		cwd = "."
	}

	fmt.Fprintf(&b, "Current working directory: %s", cwd)

	return b.String()
}

// buildToolDescriptions returns a formatted list of available tools.
// Returns empty string if no tools are registered.
func (pb *promptBuilder) buildToolDescriptions() string {
	toolNames := sdk.ListTools()
	if len(toolNames) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Available tools:\n")

	for _, name := range toolNames {
		// Try to get the tool instance for its description.
		// Use NoopConfig since we only need the definition, not execution.
		tool, err := sdk.GetTool(name, sdk.NoopConfig{})
		if err != nil {
			fmt.Fprintf(&b, "- %s\n", name)
			continue
		}

		def := tool.Definition()
		if def.Description != "" {
			fmt.Fprintf(&b, "- %s: %s\n", name, def.Description)
		} else {
			fmt.Fprintf(&b, "- %s\n", name)
		}
	}

	return strings.TrimSpace(b.String())
}

// buildSkillsUsage returns instructions for model self-invocation of skills.
func (pb *promptBuilder) buildSkillsUsage() string {
	return "<skills_usage>\n" +
		"When a skill matches the current task, load it using the read tool\n" +
		"on its <location> before taking any other action.\n" +
		"</skills_usage>"
}

// buildContextSection formats discovered context files into the prompt.
// Returns an empty string if no context files are found.
func (pb *promptBuilder) buildContextSection(files []contextFile) string {
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Project Context\n\n")

	for _, f := range files {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", f.Path, strings.TrimSpace(f.Content))
	}

	return strings.TrimSpace(b.String())
}
