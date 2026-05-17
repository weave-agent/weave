package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weave-agent/weave/sdk"
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
	propID          = "id"
	propDescription = "description"
)

func init() {
	sdk.RegisterExtension[struct{}]("subagent", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		// Register child-side inter-agent tools when running as a subagent.
		// This is done inside the factory (not init) so that the env var is
		// already set by the generated main before WireWithCore runs.
		registerMessagingTools()

		projectDir := dirFromConfig(cfg)

		cfgPath := ""
		if cfg != nil {
			cfgPath = cfg.FilePath()
		}

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, fmt.Errorf("discover agents: %w", err)
		}

		broker := NewBroker()
		mgr := newBackgroundManager(broker, cfgPath, projectDir)

		for _, agent := range agents {
			a := agent // capture loop variable
			toolName := "subagent_" + a.Name
			sdk.RegisterTool[struct{}](toolName, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
				return newSubagentTool(a, mgr, broker, cfgPath, projectDir), nil
			})
		}

		// Register background management tools
		sdk.RegisterTool[struct{}]("check_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &checkAgentTool{mgr: mgr}, nil
		})
		sdk.RegisterTool[struct{}]("await_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &awaitAgentTool{mgr: mgr}, nil
		})

		return sdk.NewExtensionFuncWithClose("subagent", func(bus sdk.Bus) error {
			mgr.setBus(bus)
			startStdinListener(bus)

			bus.On("output.redirect", func(ev sdk.Event) error {
				if payload, ok := ev.Payload.(sdk.OutputRedirectPayload); ok {
					SetStdoutWriter(payload.Writer)
				}

				return nil
			})

			return nil
		}, func() error {
			mgr.cancel()
			stopStdinListener()

			return nil
		}), nil
	})
}

// dirFromConfig derives the project directory from config.
func dirFromConfig(cfg sdk.Config) string {
	if cfg == nil {
		dir, err := os.Getwd()
		if err != nil {
			return ""
		}

		return dir
	}

	if pd := cfg.ProjectDir(); pd != "" {
		return pd
	}

	fp := cfg.FilePath()
	if fp == "" {
		dir, err := os.Getwd()
		if err != nil {
			return ""
		}

		return dir
	}

	// For .weave/settings.json, project root is the parent of .weave/
	dir := filepath.Dir(fp)
	if filepath.Base(dir) == ".weave" {
		return filepath.Dir(dir)
	}

	return dir
}

// subagentTool implements sdk.Tool for a single agent definition.
type subagentTool struct {
	agent      *AgentDef
	mgr        *backgroundManager
	broker     *Broker
	cfgPath    string
	projectDir string
}

func newSubagentTool(agent *AgentDef, mgr *backgroundManager, broker *Broker, cfgPath, projectDir string) *subagentTool {
	return &subagentTool{agent: agent, mgr: mgr, broker: broker, cfgPath: cfgPath, projectDir: projectDir}
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
					jsonType:        jsonString,
					propDescription: "Task for single mode",
				},
				paramTasks: map[string]any{
					jsonType:        jsonArray,
					"items":         map[string]any{jsonType: jsonObject},
					propDescription: "For parallel mode",
				},
				paramChain: map[string]any{
					jsonType:        jsonArray,
					"items":         map[string]any{jsonType: jsonObject},
					propDescription: "For chain mode",
				},
				paramBackground: map[string]any{
					jsonType:        jsonBool,
					propDescription: "Run in background, return agent ID immediately",
				},
				paramCWD: map[string]any{
					jsonType:        jsonString,
					propDescription: "Working directory override",
				},
			},
			"additionalProperties": false,
		},
	}
}

func (t *subagentTool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	m, err := validateMode(args)
	if err != nil {
		// Validation errors are returned as tool results, not Go errors.
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol: errors in Content, not return
	}

	cwd := ""
	if v, ok := args[paramCWD].(string); ok {
		cwd = v
	}

	if cwd != "" {
		cwd, err = resolveCWD(cwd)
		if err != nil {
			//nolint:nilerr // tool protocol: errors in Content, not return
			return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
		}
	}

	background := false
	if v, ok := args[paramBackground].(bool); ok {
		background = v
	}

	switch m {
	case modePrompt:
		prompt, ok := args[paramPrompt].(string)
		if !ok {
			return sdk.ToolResult{Content: "prompt must be a string", IsError: true}, nil
		}

		var subagentID string
		if t.agent.Messaging {
			subagentID = generateAgentID(t.agent.Name)
		}

		if background {
			if t.mgr == nil {
				return sdk.ToolResult{Content: "background manager not available", IsError: true}, nil
			}

			id, err := t.mgr.spawn(t.agent, prompt, cwd, subagentID)
			if err != nil {
				//nolint:nilerr // tool protocol: errors in Content, not return
				return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
			}

			result := map[string]any{propID: id, "status": statusRunning}

			jsonBytes, err := json.Marshal(result)
			if err != nil {
				return sdk.ToolResult{Content: fmt.Sprintf("marshal result: %v", err), IsError: true}, nil
			}

			return sdk.ToolResult{Content: string(jsonBytes)}, nil
		}

		output, err := runSubagent(ctx, t.agent, prompt, cwd, subagentID, t.broker, t.cfgPath, t.projectDir, nil)
		if err != nil {
			//nolint:nilerr // tool protocol: errors in Content, not return
			return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
		}

		return sdk.ToolResult{Content: output}, nil
	case modeParallel:
		if background {
			return sdk.ToolResult{Content: "background mode is not supported for parallel execution", IsError: true}, nil
		}

		tasks, _ := toAnySlice(args[paramTasks])

		return runParallel(ctx, t.agent, tasks, cwd, t.broker, t.cfgPath, t.projectDir)
	case modeChain:
		if background {
			return sdk.ToolResult{Content: "background mode is not supported for chain execution", IsError: true}, nil
		}

		chain, _ := toAnySlice(args[paramChain])

		return runChain(ctx, t.agent, chain, cwd, t.broker, t.cfgPath, t.projectDir)
	}

	return sdk.ToolResult{Content: fmt.Sprintf("unknown mode: %s", m), IsError: true}, nil
}

// resolveCWD resolves a cwd parameter to an absolute path and validates
// that it does not escape the current working directory.
func resolveCWD(cwd string) (string, error) {
	parentCWD, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	parentCWD = filepath.Clean(parentCWD)

	// Resolve symlinks on parentCWD so a symlink in the CWD itself cannot
	// be used to bypass the containment check.
	if resolved, evalErr := filepath.EvalSymlinks(parentCWD); evalErr == nil {
		parentCWD = resolved
	}

	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(parentCWD, cwd)
	}

	cwd = filepath.Clean(cwd)

	// Resolve symlinks before containment check to prevent a symlink
	// that points outside the project from bypassing the escape check.
	// If the path does not exist, EvalSymlinks fails; fall back to the
	// cleaned path since the subagent process will create it if needed.
	resolved := cwd
	if r, evalErr := filepath.EvalSymlinks(cwd); evalErr == nil {
		resolved = r
	}

	rel, err := filepath.Rel(parentCWD, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("cwd escapes working directory: %s", resolved)
	}

	// Re-resolve symlinks right before returning to narrow the TOCTOU
	// window between the containment check and actual use.
	if r, evalErr := filepath.EvalSymlinks(resolved); evalErr == nil {
		resolved = r
	}

	rel, err = filepath.Rel(parentCWD, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("cwd escapes working directory: %s", resolved)
	}

	return resolved, nil
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

	if arr, ok := v.([]string); ok {
		return len(arr) > 0
	}

	return false
}
