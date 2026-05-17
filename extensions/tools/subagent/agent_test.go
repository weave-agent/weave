package subagent

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgent_ValidDefinition(t *testing.T) {
	data := []byte(`---
name: explore
description: Fast codebase exploration for research and context gathering
tools: read,grep,find,ls
model: claude-haiku-4-5
sandbox: readonly
messaging: false
system: |
  You are a research agent. Explore the codebase to answer questions.
---

Optional additional system prompt instructions in markdown body.
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, "explore", agent.Name)
	assert.Equal(t, "Fast codebase exploration for research and context gathering", agent.Description)
	assert.Equal(t, []string{"read", "grep", "find", "ls"}, agent.Tools)
	assert.Equal(t, "claude-haiku-4-5", agent.Model)
	assert.Equal(t, "readonly", agent.Sandbox)
	assert.False(t, agent.Messaging)
	assert.Equal(t, "You are a research agent. Explore the codebase to answer questions.", agent.System)
	assert.Equal(t, "Optional additional system prompt instructions in markdown body.\n", agent.Body)
}

func TestParseAgent_WhitespaceSeparatedTools(t *testing.T) {
	data := []byte(`---
name: coder
description: A coding agent
tools: read edit write
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "edit", "write"}, agent.Tools)
}

func TestParseAgent_NoTools(t *testing.T) {
	data := []byte(`---
name: minimal
description: Minimal agent
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Empty(t, agent.Tools)
}

func TestParseAgent_MissingName(t *testing.T) {
	data := []byte(`---
description: Missing name
---
`)

	_, err := ParseAgent(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParseAgent_MissingFrontmatter(t *testing.T) {
	data := []byte(`This is just markdown without frontmatter.
`)

	_, err := ParseAgent(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with YAML frontmatter")
}

func TestParseAgent_UnclosedFrontmatter(t *testing.T) {
	data := []byte(`---
name: bad
`)

	_, err := ParseAgent(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frontmatter not closed")
}

func TestParseAgent_InvalidYAML(t *testing.T) {
	data := []byte(`---
name: "unclosed string
description: bad yaml
---
`)

	_, err := ParseAgent(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid YAML")
}

func TestParseAgent_BodyAsSystemFallback(t *testing.T) {
	data := []byte(`---
name: fallback
description: Uses body as system prompt
---

This body should become the system prompt.
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, "This body should become the system prompt.\n", agent.System)
}

func TestParseAgent_ExplicitSystemOverridesBodyFallback(t *testing.T) {
	data := []byte(`---
name: explicit
description: Explicit system prompt
tools: read
system: Explicit system content
---

This body should not be used as system prompt.
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, "Explicit system content", agent.System)
	assert.Equal(t, "This body should not be used as system prompt.\n", agent.Body)
}

func TestParseAgent_EmptyBodyNoSystem(t *testing.T) {
	data := []byte(`---
name: empty
description: No body or system
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Empty(t, agent.System)
	assert.Empty(t, agent.Body)
}

func TestParseAgent_BooleanMessaging(t *testing.T) {
	data := []byte(`---
name: messenger
description: Agent with messaging
tools: read
messaging: true
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.True(t, agent.Messaging)
}

func TestParseAgent_CRLFLineEndings(t *testing.T) {
	data := []byte("---\r\nname: crlf\r\ndescription: CRLF line endings\r\n---\r\n\r\nBody content.\r\n")

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, "crlf", agent.Name)
	assert.Equal(t, "Body content.\n", agent.Body)
}

func TestParseAgent_ToolsWithSpacesAroundCommas(t *testing.T) {
	data := []byte(`---
name: spaced
description: Tools with spaces around commas
tools: read , grep , find
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "grep", "find"}, agent.Tools)
}

func TestParseAgent_EmptyToolsExplicitlyBlocksAll(t *testing.T) {
	cases := []struct {
		name  string
		tools string
	}{
		{"comma_only", "tools: \",\"\n"},
		{"empty_array", "tools: []\n"},
		{"array_with_empty_string", "tools: [\"\"]\n"},
		{"empty_string", "tools: \"\"\n"},
		{"whitespace_only", "tools: \"   \"\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("---\nname: test\ndescription: Test\n" + tc.tools + "---\n")

			agent, err := ParseAgent(data)
			require.NoError(t, err)
			assert.Empty(t, agent.Tools)
			assert.NotNil(t, agent.Tools, "Tools should be non-nil empty slice, not nil")
		})
	}
}

func TestParseAgent_InvalidToolsTypeBlocksAll(t *testing.T) {
	data := []byte(`---
name: test
description: Test
tools: 123
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Empty(t, agent.Tools)
	assert.NotNil(t, agent.Tools, "Tools should be non-nil empty slice for invalid type, not nil")
}

func TestParseAgent_OmittedToolsIsNil(t *testing.T) {
	data := []byte(`---
name: test
description: Test
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Nil(t, agent.Tools, "Omitted tools field should be nil")
}

func TestParseAgent_InvalidSandbox(t *testing.T) {
	data := []byte(`---
name: badbox
description: Invalid sandbox
sandbox: invalid
---
`)

	_, err := ParseAgent(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sandbox mode")
}

func TestParseAgent_ValidSandboxModes(t *testing.T) {
	for _, mode := range []string{"off", "readonly", "ask", "auto"} {
		t.Run(mode, func(t *testing.T) {
			data := fmt.Appendf(nil, "---\nname: test\ndescription: Test\nsandbox: %s\n---\n", mode)

			agent, err := ParseAgent(data)
			require.NoError(t, err)
			assert.Equal(t, mode, agent.Sandbox)
		})
	}
}

func TestParseToolsField_WarnOnEmpty(t *testing.T) {
	var buf bytes.Buffer

	oldLogger := slog.Default()
	defer slog.SetDefault(oldLogger)

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))

	data := []byte(`---
name: warning_agent
description: Agent with no tools field
---
`)

	agent, err := ParseAgent(data)
	require.NoError(t, err)
	assert.Equal(t, "warning_agent", agent.Name)
	assert.Nil(t, agent.Tools)

	warningOutput := buf.String()
	assert.Contains(t, warningOutput, "WARN")
	assert.Contains(t, warningOutput, "warning_agent")
	assert.Contains(t, warningOutput, "no explicit tools declaration")
}
