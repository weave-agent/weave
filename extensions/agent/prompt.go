package agent

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"time"

	"weave/sdk"
)

//go:embed default-system-prompt.md
var defaultSystemPrompt string

//go:embed default-compact-prompt.md
var defaultCompactPrompt string

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
//  2. Available tools (dynamic)
//  3. Skills XML + usage instructions
//  4. Context files (CLAUDE.md/AGENTS.md)
//  5. APPEND_SYSTEM.md
//  6. Date + CWD (always injected — placed last for cache friendliness)
func (pb *promptBuilder) Build(input buildInput) string {
	var b strings.Builder

	// Layer 1: system base (default or SYSTEM.md)
	base := input.systemBase
	if base == "" {
		base = defaultSystemPrompt
	}

	b.WriteString(strings.TrimSpace(base))
	b.WriteString("\n\n")

	// Layer 2: available tools
	toolsSection := pb.buildToolDescriptions()
	if toolsSection != "" {
		b.WriteString(toolsSection)
		b.WriteString("\n\n")
	}

	// Layer 3: skills XML + usage instructions
	skillsSection := formatSkillsPrompt(input.skills)
	if skillsSection != "" {
		b.WriteString(skillsSection)
		b.WriteString("\n\n")
		b.WriteString(pb.buildSkillsUsage())
		b.WriteString("\n\n")
	}

	// Layer 4: context files
	contextSection := pb.buildContextSection(input.contextFiles)
	if contextSection != "" {
		b.WriteString(contextSection)
		b.WriteString("\n\n")
	}

	// Layer 5: APPEND_SYSTEM.md
	if input.systemAppend != "" {
		b.WriteString("<user_appended_context>\n")
		b.WriteString(sanitizeTrustBoundary(strings.TrimSpace(input.systemAppend), "user_appended_context"))
		b.WriteString("\n</user_appended_context>")
		b.WriteString("\n\n")
	}

	// Layer 6: date + CWD (last for cache friendliness)
	b.WriteString(pb.buildInjectedSection())

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
		// Pass the actual config so descriptions reflect runtime settings.
		tool, err := sdk.GetTool(name, pb.cfg)
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

// sanitizeTrustBoundary escapes wrapper tag sequences in content that could
// break out of XML trust boundaries. Prevents untrusted files from injecting
// closing tags that end the trust wrapper prematurely, or opening tags that
// restart the wrapper with different attributes.
func sanitizeTrustBoundary(content, tag string) string {
	// Escape closing tags: </tag> with optional whitespace before >.
	reClose := regexp.MustCompile(`</` + regexp.QuoteMeta(tag) + `\s*>`)
	content = reClose.ReplaceAllString(content, "&lt;/"+tag+"&gt;")

	// Escape opening tag prefix: <tag prevents any tag starting with this name.
	content = strings.ReplaceAll(content, "<"+tag, "&lt;"+tag)

	return content
}

// buildContextSection formats discovered context files into the prompt.
// Returns an empty string if no context files are found.
func (pb *promptBuilder) buildContextSection(files []contextFile) string {
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<user_context trust=\"untrusted\">\n")
	b.WriteString("# Project Context\n\n")

	for _, f := range files {
		safePath := sanitizeTrustBoundary(f.Path, "user_context")
		safeContent := sanitizeTrustBoundary(strings.TrimSpace(f.Content), "user_context")
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", safePath, safeContent)
	}

	b.WriteString("</user_context>")

	return b.String()
}
