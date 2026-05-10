package subagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weave/sdk"
)

const (
	jsonType   = "type"
	jsonObject = "object"
	jsonArray  = "array"
	jsonString = "string"
	jsonBool   = "boolean"
)

const (
	paramPrompt     = "prompt"
	paramTasks      = "tasks"
	paramChain      = "chain"
	paramBackground = "background"
	paramCWD        = "cwd"
)

func init() {
	sdk.RegisterExtension("subagent", func(cfg sdk.Config) (sdk.Extension, error) {
		projectDir := dirFromConfig(cfg)

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, fmt.Errorf("discover agents: %w", err)
		}

		for _, agent := range agents {
			a := agent // capture loop variable
			toolName := "subagent_" + a.Name
			sdk.RegisterTool(toolName, func(sdk.Config) (sdk.Tool, error) {
				return newSubagentTool(a), nil
			})
		}

		return sdk.NewExtensionFunc("subagent", nil), nil
	})
}

// dirFromConfig derives the project directory from config.
func dirFromConfig(cfg sdk.Config) string {
	if cfg == nil {
		dir, _ := os.Getwd()

		return dir
	}

	fp := cfg.FilePath()
	if fp == "" {
		dir, _ := os.Getwd()

		return dir
	}

	// For .weave/config.yaml, project root is the parent of .weave/
	if strings.Contains(fp, string(filepath.Separator)+".weave"+string(filepath.Separator)) {
		return filepath.Dir(filepath.Dir(fp))
	}

	// For .weave.yaml, project root is the directory containing it
	return filepath.Dir(fp)
}

// subagentTool implements sdk.Tool for a single agent definition.
type subagentTool struct {
	agent *AgentDef
}

func newSubagentTool(agent *AgentDef) *subagentTool {
	return &subagentTool{agent: agent}
}

func (t *subagentTool) Name() string {
	return "subagent_" + t.agent.Name
}

func (t *subagentTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        t.Name(),
		Description: t.agent.Description,
		Parameters: map[string]any{
			jsonType: jsonObject,
			"properties": map[string]any{
				paramPrompt: map[string]any{
					jsonType:      jsonString,
					"description": "Task for single mode",
				},
				paramTasks: map[string]any{
					jsonType:      jsonArray,
					"items":       map[string]any{jsonType: jsonObject},
					"description": "For parallel mode",
				},
				paramChain: map[string]any{
					jsonType:      jsonArray,
					"items":       map[string]any{jsonType: jsonObject},
					"description": "For chain mode",
				},
				paramBackground: map[string]any{
					jsonType:      jsonBool,
					"description": "Run in background, return agent ID immediately",
				},
				paramCWD: map[string]any{
					jsonType:      jsonString,
					"description": "Working directory override",
				},
			},
		},
	}
}

func (t *subagentTool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	m, err := validateMode(args)
	if err != nil {
		// Validation errors are returned as tool results, not Go errors.
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol: errors in Content, not return
	}

	_ = m

	// Stub: actual execution implemented in later tasks.
	return sdk.ToolResult{Content: fmt.Sprintf("subagent %s: execution not yet implemented (mode: %s)", t.agent.Name, m)}, nil
}

// mode represents which execution mode was requested.
type mode string

const (
	modePrompt   mode = "prompt"
	modeParallel mode = "parallel"
	modeChain    mode = "chain"
)

// validateMode checks that exactly one of prompt, tasks, or chain is provided.
func validateMode(args map[string]any) (mode, error) {
	hasPrompt := hasNonEmptyString(args, paramPrompt)
	hasTasks := hasNonEmptyArray(args, paramTasks)
	hasChain := hasNonEmptyArray(args, paramChain)

	count := 0
	if hasPrompt {
		count++
	}

	if hasTasks {
		count++
	}

	if hasChain {
		count++
	}

	if count == 0 {
		return "", errors.New("error: exactly one of prompt, tasks, or chain is required")
	}

	if count > 1 {
		return "", errors.New("error: prompt, tasks, and chain are mutually exclusive")
	}

	if hasPrompt {
		return modePrompt, nil
	}

	if hasTasks {
		return modeParallel, nil
	}

	return modeChain, nil
}

func hasNonEmptyString(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}

	s, ok := v.(string)

	return ok && s != ""
}

func hasNonEmptyArray(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}

	if arr, ok := v.([]any); ok {
		return len(arr) > 0
	}

	if arr, ok := v.([]map[string]any); ok {
		return len(arr) > 0
	}

	return false
}
