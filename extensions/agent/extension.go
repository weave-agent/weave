package agent

//go:generate moq -fmt goimports -stub -skip-ensure -pkg agent -out mock_test.go ../../sdk Bus Provider Tool UI

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"weave/sdk"
	"weave/sdk/model"
	"weave/settings"
)

const defaultProviderName = "anthropic"

// CompactionConfig controls automatic context compaction behavior.
type CompactionConfig struct {
	Enabled          bool   `json:"enabled" default:"true" description:"Enable auto-compaction"`
	ReserveTokens    int    `json:"reserve_tokens" default:"16384" description:"Tokens reserved for model response"`
	KeepRecentTokens int    `json:"keep_recent_tokens" default:"20000" description:"Recent tokens to keep (not summarized)"`
	Model            string `json:"model" default:"" description:"Model for summary generation (empty = current model)"`
}

// AgentExtension owns the entire conversation lifecycle:
// prompt assembly, turn loop, tool execution, skill discovery, and context file loading.
type AgentExtension struct {
	cfg                 sdk.Config
	providerName        string
	modelName           string
	singleTurn          bool
	thinkingLevel       model.ThinkingLevel
	skillDiscoveryPaths []string // override for testing
	compactionCfg       CompactionConfig

	fileOps *fileOperations

	resumed   bool
	sessionID string

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func init() {
	sdk.RegisterExtension("agent", func(cfg sdk.Config, ps sdk.PreferenceStore, cc CompactionConfig) (sdk.Extension, error) {
		return NewAgentExtension(cfg, ps, cc)
	})
}

func NewAgentExtension(cfg sdk.Config, ps sdk.PreferenceStore, cc CompactionConfig) (*AgentExtension, error) {
	provider := resolveProviderName(os.Getenv("WEAVE_PROVIDER"), ps)

	modelName := resolveModelName(ps)
	if modelName != "" {
		if _, ok := model.GetModelForProvider(modelName, provider); !ok {
			// Not found for this provider. If it exists for any other provider,
			// clear it. If it's unregistered (custom model), keep it.
			if _, exists := model.GetModel(modelName); exists {
				modelName = ""
			}
		}
	}

	return &AgentExtension{
		cfg:           cfg,
		providerName:  provider,
		modelName:     modelName,
		singleTurn:    os.Getenv("WEAVE_SINGLE_TURN") == "1",
		thinkingLevel: resolveThinkingLevel(ps),
		compactionCfg: cc,
	}, nil
}

func (a *AgentExtension) Name() string { return "agent" }

func (a *AgentExtension) Subscribe(bus sdk.Bus) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return errors.New("agent: Subscribe called twice without Close")
	}

	ctx, cancel := context.WithCancel(context.Background())

	promptCh := make(chan sdk.Event, 64)
	steerCh := make(chan sdk.Event, 64)
	followupCh := make(chan sdk.Event, 64)
	interruptCh := make(chan sdk.Event, 64)
	modelChangeCh := make(chan sdk.Event, 64)
	thinkingCh := make(chan sdk.Event, 64)
	sessionResumeCh := make(chan sdk.Event, 64)
	authLogoutCh := make(chan sdk.Event, 64)

	bus.OnAll(func(ev sdk.Event) error {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var ch chan sdk.Event

		switch ev.Topic {
		case TopicPrompt:
			ch = promptCh
		case TopicSteer:
			ch = steerCh
		case TopicFollowup:
			ch = followupCh
		case TopicInterrupt:
			ch = interruptCh
		case TopicModelChange:
			ch = modelChangeCh
		case TopicThinkingChange:
			ch = thinkingCh
		case TopicSessionResume:
			ch = sessionResumeCh
		case TopicAuthLogout:
			ch = authLogoutCh
		}

		if ch != nil {
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		}

		return nil
	})

	a.cancel = cancel
	a.done = make(chan struct{})

	go a.run(ctx, bus, promptCh, steerCh, followupCh, interruptCh, modelChangeCh, thinkingCh, sessionResumeCh, authLogoutCh)

	a.registerSkillCommands(bus)

	a.mu.Unlock()

	return nil
}

func (a *AgentExtension) Close() error {
	a.mu.Lock()
	cancel := a.cancel
	done := a.done
	a.cancel = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	return nil
}

// projectDir returns the project directory from config, or derives it from the
// config file path.
func (a *AgentExtension) projectDir() string {
	if a.cfg == nil {
		return ""
	}

	if pd := a.cfg.ProjectDir(); pd != "" {
		return pd
	}

	fp := a.cfg.FilePath()
	if fp == "" {
		return ""
	}

	dir := filepath.Dir(fp)
	if filepath.Base(dir) == ".weave" {
		dir = filepath.Dir(dir)
	}

	return dir
}

// globalConfigDir returns the global config directory (~/.weave).
func globalConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	return filepath.Join(home, ".weave")
}

// resolveSkillPaths returns the skill discovery paths in precedence order:
// project > global > extension-bundled.
func (a *AgentExtension) resolveSkillPaths() []string {
	if len(a.skillDiscoveryPaths) > 0 {
		return a.skillDiscoveryPaths
	}

	projectDir := a.projectDir()
	globalDir := globalConfigDir()

	var paths []string

	if projectDir != "" {
		paths = append(paths, filepath.Join(projectDir, ".weave", "skills"))
	}

	if globalDir != "" {
		paths = append(paths, filepath.Join(globalDir, "skills"))
	}

	paths = append(paths, discoverExtensionSkills(projectDir, globalDir)...)

	return paths
}

// registerSkillCommands discovers skills and registers /skill:<name> slash
// commands with the TUI. Silently skipped in headless mode.
func (a *AgentExtension) registerSkillCommands(bus sdk.Bus) {
	ui, err := sdk.GetUI("tui")
	if err != nil {
		return // headless mode: no TUI available
	}

	skills := discoverSkills(a.resolveSkillPaths()...)

	for i := range skills {
		skill := skills[i]
		cmdName := "/skill:" + skill.Name
		ui.RegisterCommand(cmdName, makeSkillHandler(skill, bus))
	}
}

// resolveProviderName picks the initial provider using priority:
//  1. WEAVE_PROVIDER env var (explicit user override)
//  2. settings.json "provider" field (persisted user preference)
//  3. alphabetically first registered provider (sdk.ListProviders()[0])
//  4. "anthropic" (ultimate fallback)
func resolveProviderName(envProvider string, ps sdk.PreferenceStore) string {
	if envProvider != "" {
		return envProvider
	}

	var prefs struct {
		Provider string `json:"provider"`
	}

	if ps != nil && ps.Preferences(&prefs) == nil && prefs.Provider != "" {
		return prefs.Provider
	}

	if providers := sdk.ListProviders(); len(providers) > 0 {
		return providers[0]
	}

	return defaultProviderName
}

// resolveModelName reads the persisted model from settings. Returns empty
// string when no model is set, which lets the provider use its default.
func resolveModelName(ps sdk.PreferenceStore) string {
	if ps == nil {
		return ""
	}

	var prefs struct {
		Model string `json:"model,omitempty"`
	}

	if ps.Preferences(&prefs) == nil {
		return prefs.Model
	}

	return ""
}

// resolveThinkingLevel reads the persisted thinking level from settings,
// falling back to WEAVE_THINKING_LEVEL env var, then medium.
func resolveThinkingLevel(ps sdk.PreferenceStore) model.ThinkingLevel {
	if ps != nil {
		var prefs struct {
			ThinkingLevel string `json:"thinking_level,omitempty"`
		}

		if ps.Preferences(&prefs) == nil && prefs.ThinkingLevel != "" {
			if lvl, err := model.ParseThinkingLevel(prefs.ThinkingLevel); err == nil {
				return lvl
			}
		}
	}

	return settings.DefaultThinkingLevel()
}
