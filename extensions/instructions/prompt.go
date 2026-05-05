package instructions

import (
	"fmt"
	"strings"
)

func formatInstructionsPrompt(contextFiles []ContextFile, systemBase, systemAppend string) string {
	var b strings.Builder

	if systemBase != "" {
		b.WriteString(strings.TrimSpace(systemBase))
		b.WriteString("\n\n")
	}

	if len(contextFiles) > 0 {
		b.WriteString("# Project Context\n\n")

		for _, f := range contextFiles {
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", f.Path, strings.TrimSpace(f.Content))
		}
	}

	if systemAppend != "" {
		b.WriteString(strings.TrimSpace(systemAppend))
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
