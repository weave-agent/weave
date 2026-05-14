package subagent

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"weave/sdk"
)

//go:embed agents/*.md
var builtinAgentsFS embed.FS

// loadBuiltinAgents loads agent definitions from the embedded agents directory.
func loadBuiltinAgents() ([]*AgentDef, error) {
	entries, err := builtinAgentsFS.ReadDir("agents")
	if err != nil {
		return nil, fmt.Errorf("read embedded agents dir: %w", err)
	}

	var agents []*AgentDef

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := builtinAgentsFS.ReadFile(path.Join("agents", entry.Name()))
		if err != nil {
			sdk.Logger("subagent").Warn("failed to read embedded agent", "name", entry.Name(), "error", err)
			continue
		}

		agent, err := ParseAgent(data)
		if err != nil {
			sdk.Logger("subagent").Warn("failed to parse embedded agent", "name", entry.Name(), "error", err)
			continue
		}

		agents = append(agents, agent)
	}

	return agents, nil
}

// discoverFilesystemAgents walks dir for .md files and parses them as agent definitions.
// Missing directories are silently ignored. Invalid files are logged as warnings and skipped.
func discoverFilesystemAgents(dir string) ([]*AgentDef, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("stat %q: %w", dir, err)
	}

	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %q: %w", dir, err)
	}

	var agents []*AgentDef

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		agentPath := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(agentPath)
		if err != nil {
			sdk.Logger("subagent").Warn("failed to read agent file", "path", agentPath, "error", err)
			continue
		}

		agent, err := ParseAgent(data)
		if err != nil {
			sdk.Logger("subagent").Warn("failed to parse agent file", "path", agentPath, "error", err)
			continue
		}

		agents = append(agents, agent)
	}

	return agents, nil
}

// DiscoverAgents discovers agent definitions from three sources, in order of precedence:
//  1. Built-in agents (embedded in the binary)
//  2. Global user agents (~/.weave/agents/)
//  3. Project-local agents (.weave/agents/)
//
// Agents with the same name are overridden by higher-precedence sources:
// project > global > built-in.
func DiscoverAgents(projectDir string) ([]*AgentDef, error) {
	builtins, err := loadBuiltinAgents()
	if err != nil {
		return nil, fmt.Errorf("load built-in agents: %w", err)
	}

	byName := make(map[string]*AgentDef, len(builtins))
	for _, a := range builtins {
		byName[a.Name] = a
	}

	home, err := os.UserHomeDir()
	if err == nil {
		globalAgents, err := discoverFilesystemAgents(filepath.Join(home, ".weave", "agents"))
		if err != nil {
			return nil, fmt.Errorf("discover global agents: %w", err)
		}

		for _, a := range globalAgents {
			byName[a.Name] = a
		}
	}

	if projectDir != "" {
		projectAgents, err := discoverFilesystemAgents(filepath.Join(projectDir, ".weave", "agents"))
		if err != nil {
			return nil, fmt.Errorf("discover project agents: %w", err)
		}

		for _, a := range projectAgents {
			byName[a.Name] = a
		}
	}

	agents := make([]*AgentDef, 0, len(byName))
	for _, a := range byName {
		agents = append(agents, a)
	}

	return agents, nil
}
