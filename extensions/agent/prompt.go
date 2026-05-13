package agent

import (
	"fmt"
	"strings"

	"weave/sdk"
)

// promptBuilder assembles the system prompt from multiple layers:
// default prompt or SYSTEM.md, date/CWD, tool descriptions, skills, context files, APPEND_SYSTEM.md.
type promptBuilder struct {
	cfg sdk.Config
}

func newPromptBuilder(cfg sdk.Config) *promptBuilder {
	return &promptBuilder{cfg: cfg}
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

// formatInstructionsPrompt assembles system prompt components in order:
// system base, context files, system append.
func (pb *promptBuilder) formatInstructionsPrompt(files []contextFile, systemBase, systemAppend string) string {
	var b strings.Builder

	if systemBase != "" {
		b.WriteString(strings.TrimSpace(systemBase))
		b.WriteString("\n\n")
	}

	contextSection := pb.buildContextSection(files)
	if contextSection != "" {
		b.WriteString(contextSection)
		b.WriteString("\n\n")
	}

	if systemAppend != "" {
		b.WriteString(strings.TrimSpace(systemAppend))
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
