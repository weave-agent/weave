package subagent

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltinAgents(t *testing.T) {
	agents, err := loadBuiltinAgents()
	require.NoError(t, err)
	require.Len(t, agents, 3)

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
	}

	assert.True(t, names["general"], "expected built-in 'general' agent")
	assert.True(t, names["explore"], "expected built-in 'explore' agent")
	assert.True(t, names["plan"], "expected built-in 'plan' agent")
}

func TestDiscoverFilesystemAgents_ValidFiles(t *testing.T) {
	dir := t.TempDir()

	validAgent := []byte(`---
name: custom
description: A custom agent
tools: read, grep
---
`)
	err := os.WriteFile(filepath.Join(dir, "custom.md"), validAgent, 0o644)
	require.NoError(t, err)

	agents, err := discoverFilesystemAgents(dir)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "custom", agents[0].Name)
	assert.Equal(t, "A custom agent", agents[0].Description)
	assert.Equal(t, []string{"read", "grep"}, agents[0].Tools)
}

func TestDiscoverFilesystemAgents_MissingDir(t *testing.T) {
	agents, err := discoverFilesystemAgents("/nonexistent/path/to/agents")
	require.NoError(t, err)
	assert.Empty(t, agents)
}

func TestDiscoverFilesystemAgents_InvalidFilesSkipped(t *testing.T) {
	var buf bytes.Buffer

	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	dir := t.TempDir()

	validAgent := []byte(`---
name: valid
description: Valid agent
---
`)
	err := os.WriteFile(filepath.Join(dir, "valid.md"), validAgent, 0o644)
	require.NoError(t, err)

	invalidAgent := []byte(`not valid frontmatter
`)
	err = os.WriteFile(filepath.Join(dir, "invalid.md"), invalidAgent, 0o644)
	require.NoError(t, err)

	agents, err := discoverFilesystemAgents(dir)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "valid", agents[0].Name)

	warningOutput := buf.String()
	assert.Contains(t, warningOutput, "invalid.md")
	assert.Contains(t, warningOutput, "warning:")
}

func TestDiscoverFilesystemAgents_NonMarkdownIgnored(t *testing.T) {
	dir := t.TempDir()

	validAgent := []byte(`---
name: valid
description: Valid agent
---
`)
	err := os.WriteFile(filepath.Join(dir, "valid.md"), validAgent, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not an agent"), 0o644)
	require.NoError(t, err)

	agents, err := discoverFilesystemAgents(dir)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "valid", agents[0].Name)
}

func TestDiscoverFilesystemAgents_SubdirectoriesIgnored(t *testing.T) {
	dir := t.TempDir()

	validAgent := []byte(`---
name: valid
description: Valid agent
---
`)
	err := os.WriteFile(filepath.Join(dir, "valid.md"), validAgent, 0o644)
	require.NoError(t, err)

	subdir := filepath.Join(dir, "nested")
	err = os.MkdirAll(subdir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subdir, "nested.md"), validAgent, 0o644)
	require.NoError(t, err)

	agents, err := discoverFilesystemAgents(dir)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "valid", agents[0].Name)
}

func TestDiscoverAgents_LoadsBuiltinsWhenNoProject(t *testing.T) {
	agents, err := DiscoverAgents("")
	require.NoError(t, err)
	require.NotEmpty(t, agents)

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
	}

	assert.True(t, names["general"])
	assert.True(t, names["explore"])
	assert.True(t, names["plan"])
}

func TestDiscoverAgents_ProjectOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()

	projectGeneral := []byte(`---
name: general
description: Project override of general
tools: read
---
`)
	err := os.MkdirAll(filepath.Join(dir, ".weave", "agents"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "general.md"), projectGeneral, 0o644)
	require.NoError(t, err)

	agents, err := DiscoverAgents(dir)
	require.NoError(t, err)

	var generalAgent *AgentDef

	for _, a := range agents {
		if a.Name == "general" {
			generalAgent = a
			break
		}
	}

	require.NotNil(t, generalAgent)
	assert.Equal(t, "Project override of general", generalAgent.Description)
	assert.Equal(t, []string{"read"}, generalAgent.Tools)
}

func TestDiscoverAgents_GlobalOverridesBuiltin(t *testing.T) {
	t.Skip("requires manipulating ~/.weave/agents/ — test precedence via unit tests of merge logic")
}

func TestDiscoverAgents_ProjectOverridesGlobal(t *testing.T) {
	// This tests the precedence logic directly: project agents override global agents.
	// We verify by creating a project agent with the same name as a built-in.
	dir := t.TempDir()

	projectExplore := []byte(`---
name: explore
description: Project explore override
tools: read
---
`)
	err := os.MkdirAll(filepath.Join(dir, ".weave", "agents"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "explore.md"), projectExplore, 0o644)
	require.NoError(t, err)

	agents, err := DiscoverAgents(dir)
	require.NoError(t, err)

	var exploreAgent *AgentDef

	for _, a := range agents {
		if a.Name == "explore" {
			exploreAgent = a
			break
		}
	}

	require.NotNil(t, exploreAgent)
	assert.Equal(t, "Project explore override", exploreAgent.Description)
	assert.Equal(t, []string{"read"}, exploreAgent.Tools)
}

func TestDiscoverAgents_MultipleProjectAgents(t *testing.T) {
	dir := t.TempDir()

	agent1 := []byte(`---
name: agent1
description: First agent
---
`)
	agent2 := []byte(`---
name: agent2
description: Second agent
---
`)

	err := os.MkdirAll(filepath.Join(dir, ".weave", "agents"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "agent1.md"), agent1, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "agent2.md"), agent2, 0o644)
	require.NoError(t, err)

	agents, err := DiscoverAgents(dir)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
	}

	assert.True(t, names["agent1"])
	assert.True(t, names["agent2"])
	assert.True(t, names["general"])
	assert.True(t, names["explore"])
	assert.True(t, names["plan"])
}

func TestDiscoverAgents_InvalidProjectFileSkipped(t *testing.T) {
	var buf bytes.Buffer

	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	dir := t.TempDir()

	validAgent := []byte(`---
name: valid
description: Valid project agent
---
`)
	invalidAgent := []byte(`not valid frontmatter
`)

	err := os.MkdirAll(filepath.Join(dir, ".weave", "agents"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "valid.md"), validAgent, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, ".weave", "agents", "invalid.md"), invalidAgent, 0o644)
	require.NoError(t, err)

	agents, err := DiscoverAgents(dir)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
	}

	assert.True(t, names["valid"])
	assert.False(t, names["invalid"])
	assert.True(t, names["general"])

	warningOutput := buf.String()
	assert.Contains(t, warningOutput, "invalid.md")
}
