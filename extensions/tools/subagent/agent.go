package subagent

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentDef defines a subagent's capabilities and behavior.
type AgentDef struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Model       string   `yaml:"model"`
	Sandbox     string   `yaml:"sandbox"`
	Messaging   bool     `yaml:"messaging"`
	System      string   `yaml:"system"`
	Body        string   // markdown body after frontmatter
}

// agentFrontmatter mirrors AgentDef for YAML unmarshaling.
type agentFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tools       string `yaml:"tools"`
	Model       string `yaml:"model"`
	Sandbox     string `yaml:"sandbox"`
	Messaging   bool   `yaml:"messaging"`
	System      string `yaml:"system"`
}

// ParseAgent parses an agent definition from markdown with YAML frontmatter.
// Returns the populated AgentDef or an error if the frontmatter is invalid.
func ParseAgent(data []byte) (*AgentDef, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")

	var fm agentFrontmatter

	if !strings.HasPrefix(content, "---\n") {
		return nil, errors.New("agent definition must start with YAML frontmatter (---)")
	}

	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return nil, errors.New("agent definition frontmatter not closed (missing ---)")
	}

	fmText := content[4 : 4+end]
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	body := content[4+end+5:]
	body = strings.TrimLeft(body, "\n")

	if fm.Name == "" {
		return nil, errors.New("agent name is required in frontmatter")
	}

	var tools []string
	if fm.Tools != "" {
		tools = parseTools(fm.Tools)
	}

	// Use body as system prompt fallback if no explicit system field.
	system := fm.System
	if system == "" && body != "" {
		system = body
	}

	return &AgentDef{
		Name:        fm.Name,
		Description: fm.Description,
		Tools:       tools,
		Model:       fm.Model,
		Sandbox:     fm.Sandbox,
		Messaging:   fm.Messaging,
		System:      system,
		Body:        body,
	}, nil
}

// parseTools splits a comma or whitespace separated tool list.
func parseTools(s string) []string {
	// First try comma-separated.
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")

		var out []string

		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}

		return out
	}
	// Fallback to whitespace-separated.
	return strings.Fields(s)
}
