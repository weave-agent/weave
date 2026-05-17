package subagent

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"weave/sdk"
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
	Tools       any    `yaml:"tools"`
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

	// Find the closing --- delimiter. Use strings.Index sequentially to handle
	// cases where --- appears inside the frontmatter body.
	var end int

	searchStart := 4
	for {
		idx := strings.Index(content[searchStart:], "\n---\n")
		if idx < 0 {
			return nil, errors.New("agent definition frontmatter not closed (missing ---)")
		}

		end = searchStart + idx
		// Verify that what precedes it is valid YAML by trying to unmarshal.
		// If it fails, continue searching for the next ---.
		candidate := content[4:end]
		if err := yaml.Unmarshal([]byte(candidate), &fm); err != nil {
			searchStart = end + 1
			// If there are no more --- delimiters, the YAML is genuinely invalid.
			if !strings.Contains(content[searchStart:], "\n---\n") {
				return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
			}

			continue
		}

		break
	}

	fmText := content[4:end]
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	body := content[end+5:]
	body = strings.TrimLeft(body, "\n")

	if fm.Name == "" {
		return nil, errors.New("agent name is required in frontmatter")
	}

	tools := parseToolsField(fm.Tools)

	if tools == nil {
		sdk.Logger("subagent").Warn("agent has no explicit tools declaration", "name", fm.Name, "hint", "add a 'tools' field to explicitly declare which tools this agent should use; omitting 'tools' grants access to all available tools")
	}

	if fm.Sandbox != "" {
		allowed := map[string]bool{"off": true, "readonly": true, "ask": true, "auto": true}
		if !allowed[fm.Sandbox] {
			return nil, fmt.Errorf("invalid sandbox mode %q, must be one of: off, readonly, ask, auto", fm.Sandbox)
		}
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

		out := make([]string, 0, len(parts))

		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}

		return out
	}
	// Fallback to whitespace-separated.
	result := strings.Fields(s)
	if result == nil {
		return []string{}
	}

	return result
}

// parseToolsField normalizes the Tools frontmatter field which may be a
// string or a YAML array.
func parseToolsField(v any) []string {
	if v == nil {
		return nil
	}

	if s, ok := v.(string); ok {
		return parseTools(s)
	}

	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))

		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}

		return out
	}

	return []string{}
}
