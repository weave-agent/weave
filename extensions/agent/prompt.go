package agent

import "weave/sdk"

// promptBuilder assembles the system prompt from multiple layers:
// default prompt or SYSTEM.md, date/CWD, tool descriptions, skills, context files, APPEND_SYSTEM.md.
type promptBuilder struct {
	cfg sdk.Config
}

func newPromptBuilder(cfg sdk.Config) *promptBuilder {
	return &promptBuilder{cfg: cfg}
}
