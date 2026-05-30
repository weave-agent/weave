package wire

//go:generate moq -fmt goimports -stub -skip-ensure -pkg wire -out wire_mock_test.go ../../sdk Bus Provider

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/weave-agent/weave/internal/extmanage"
	"github.com/weave-agent/weave/internal/filemut"
	"github.com/weave-agent/weave/internal/filetracker"
	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"
)

const defaultAgentLoop = "agent"

type CoreWireConfig struct {
	AgentLoop  string
	SingleTurn bool
	Continue   bool
	Resume     string
}

type Wired struct {
	extensions []sdk.Extension
	bus        sdk.Bus
	runtime    sdk.ExtensionContext
}

type runtimeExtensionAdapter struct {
	name    string
	runtime sdk.RuntimeExtension
	ctx     sdk.ExtensionContext
}

func (e runtimeExtensionAdapter) Name() string { return e.name }

func (e runtimeExtensionAdapter) Subscribe(sdk.Bus) error {
	if e.runtime == nil {
		return nil
	}

	if err := e.runtime.Register(e.ctx); err != nil {
		return fmt.Errorf("register runtime extension %q: %w", e.name, err)
	}

	return nil
}

func (e runtimeExtensionAdapter) Close() error {
	if closer, ok := e.runtime.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close runtime extension %q: %w", e.name, err)
		}
	}

	return nil
}

func resolveExtensions(extNames []string, cfg sdk.Config, runtime sdk.ExtensionContext) ([]sdk.Extension, error) {
	if cfg == nil {
		cfg = sdk.FilePathConfig("")
	}

	exts := make([]sdk.Extension, 0, len(extNames))

	for _, name := range extNames {
		runtimeExt, err := sdk.GetRuntimeExtension(name, cfg)
		if err != nil {
			// If the name is not registered as an extension, it may be a tool,
			// provider, or UI extension that wires through its own registry.
			// Silently skip those as well as truly unknown names.
			if !sdk.ExtensionRegistered(name) {
				continue
			}

			for _, v := range slices.Backward(exts) {
				_ = v.Close()
			}

			return nil, fmt.Errorf("wire: %w", err)
		}

		exts = append(exts, runtimeExtensionAdapter{name: name, runtime: runtimeExt, ctx: runtime})
	}

	return exts, nil
}

func subscribeExtensions(exts []sdk.Extension, bus sdk.Bus) error {
	for i, ext := range exts {
		if err := ext.Subscribe(bus); err != nil {
			for j := range slices.Backward(exts[:i]) {
				_ = exts[j].Close()
			}

			return fmt.Errorf("wire: subscribe %s: %w", ext.Name(), err)
		}
	}

	return nil
}

func prepareExtensions(extNames []string, cfg sdk.Config, runtime sdk.ExtensionContext) ([]sdk.Extension, error) {
	exts, err := resolveExtensions(extNames, cfg, runtime)
	if err != nil {
		return nil, err
	}

	// Check auth status for all registered providers BEFORE subscribing
	// extensions. The TUI model initializes during Subscribe and queries
	// ProviderHasAuth, so the registry must be populated first.
	for _, name := range sdk.ListProviders() {
		hasAuth, err := sdk.CheckProviderAuth(name)
		if err != nil {
			slog.Warn("check auth failed", "provider", name, "error", err)
		}

		model.SetProviderAuth(name, hasAuth)
	}

	// Register the first extension that implements SessionStore.
	for _, ext := range exts {
		if ss, ok := ext.(sdk.SessionStore); ok {
			sdk.SetSessionStore(ss)

			break
		}

		runtimeAdapter, ok := ext.(runtimeExtensionAdapter)
		if !ok {
			continue
		}

		if ss, ok := runtimeAdapter.runtime.(sdk.SessionStore); ok {
			sdk.SetSessionStore(ss)

			break
		}

		if legacy, ok := runtimeAdapter.runtime.(interface{ LegacyExtension() sdk.Extension }); ok {
			if ss, ok := legacy.LegacyExtension().(sdk.SessionStore); ok {
				sdk.SetSessionStore(ss)

				break
			}
		}
	}

	return exts, nil
}

func WireExtensions(extNames []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
	runtime := newRuntimeContext(bus, cfg)

	exts, err := prepareExtensions(extNames, cfg, runtime)
	if err != nil {
		return nil, err
	}

	if err := subscribeExtensions(exts, bus); err != nil {
		return nil, err
	}

	return &Wired{extensions: exts, bus: bus, runtime: runtime}, nil
}

func resolveSession(continueFlag bool, resumeID string, bus sdk.Bus, cfg sdk.Config) (string, []sdk.Message, error) {
	store := sdk.GetSessionStore()
	if store == nil {
		return "", nil, errors.New("no session store available")
	}

	sessionID, messages, err := resolveSessionFromStore(store, continueFlag, resumeID, continueCWD(cfg))
	if err != nil {
		return "", nil, err
	}

	if sessionID != "" {
		payload := sdk.SessionResumePayload{
			SessionID: sessionID,
			Messages:  messages,
		}
		sdk.SetInitialSession(payload)
		bus.Publish(sdk.NewEvent("session.resume", payload))
	}

	return sessionID, messages, nil
}

func resolveSessionFromStore(store sdk.SessionStore, continueFlag bool, resumeID, cwd string) (string, []sdk.Message, error) {
	if resumeID != "" {
		messages, err := store.LoadHistory(resumeID)
		if err != nil {
			return "", nil, fmt.Errorf("load session %s: %w", resumeID, err)
		}

		return resumeID, messages, nil
	}

	if continueFlag {
		return resolveContinueSession(store, cwd)
	}

	return "", nil, nil
}

func resolveContinueSession(store sdk.SessionStore, cwd string) (string, []sdk.Message, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return "", nil, fmt.Errorf("list sessions: %w", err)
	}

	if cwd != "" {
		sessions = sessionsForCWD(sessions, cwd)
	}

	if len(sessions) == 0 {
		if cwd != "" {
			return "", nil, fmt.Errorf("no sessions found for %s", cwd)
		}

		return "", nil, errors.New("no sessions found")
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	sessionID := sessions[0].ID

	messages, err := store.LoadHistory(sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("load session %s: %w", sessionID, err)
	}

	return sessionID, messages, nil
}

func continueCWD(cfg sdk.Config) string {
	if cfg != nil {
		if projectDir := cfg.ProjectDir(); projectDir != "" {
			return filepath.Clean(projectDir)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return filepath.Clean(cwd)
}

func sessionsForCWD(sessions []sdk.SessionInfo, cwd string) []sdk.SessionInfo {
	cleanCWD := filepath.Clean(cwd)
	filtered := make([]sdk.SessionInfo, 0, len(sessions))

	for _, session := range sessions {
		if session.CWD == "" {
			continue
		}

		if filepath.Clean(session.CWD) == cleanCWD {
			filtered = append(filtered, session)
		}
	}

	return filtered
}

func WireWithCore(core CoreWireConfig, optExts []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
	sdk.ResetInitialSession()

	if sdk.GetFileTracker() == nil {
		sdk.SetFileTracker(filetracker.New())
	}

	if sdk.GetFileMutex() == nil {
		sdk.SetFileMutex(filemut.New())
	}

	if err := validateCore(core); err != nil {
		return nil, fmt.Errorf("wire: %w", err)
	}

	if !sdk.ExtensionRegistered(core.AgentLoop) {
		return nil, fmt.Errorf("wire: agent-loop extension %q is not registered", core.AgentLoop)
	}

	cleanup := setSingleTurnEnv(core.SingleTurn)
	defer cleanup()

	extNames := mergeCoreAndOptional(core.AgentLoop, optExts)

	// Wire bus subscriptions for tools and other non-extension code before
	// resolving extensions, so subscriptions are active when extensions
	// publish their registration events (e.g. sandbox.registered).
	sdk.InvokeBusSubscribers(bus)

	runtime := newRuntimeContext(bus, cfg)

	exts, err := prepareExtensions(extNames, cfg, runtime)
	if err != nil {
		return nil, err
	}

	if core.Continue || core.Resume != "" {
		if _, _, err := resolveSession(core.Continue, core.Resume, bus, cfg); err != nil {
			if cfg != nil && cfg.IsHeadless() {
				return nil, fmt.Errorf("session resume: %w", err)
			}

			slog.Warn("session resume failed", "error", err)
		}
	}

	if err := subscribeExtensions(exts, bus); err != nil {
		return nil, err
	}

	wired := &Wired{extensions: exts, bus: bus, runtime: runtime}

	go func() {
		bus.Publish(sdk.NewEvent("app.started", nil))

		if cfg == nil || !cfg.IsHeadless() {
			extmanage.FireUpdateCheck(bus)
		}
	}()

	return wired, nil
}

func newRuntimeContext(bus sdk.Bus, cfg sdk.Config) sdk.ExtensionContext {
	if cfg == nil {
		cfg = sdk.FilePathConfig("")
	}

	return sdk.NewExtensionContext(sdk.RuntimeContextOptions{
		Bus:    bus,
		Config: cfg,
	})
}

func (w *Wired) Runtime() sdk.ExtensionContext {
	if w == nil || w.runtime == nil {
		return sdk.NewExtensionContext(sdk.RuntimeContextOptions{})
	}

	return w.runtime
}

func (w *Wired) Close() error {
	var errs []error

	for _, v := range slices.Backward(w.extensions) {
		if err := v.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close: %w", errors.Join(errs...))
	}

	return nil
}

func validateCore(core CoreWireConfig) error {
	if core.AgentLoop == "" {
		return errors.New("agent-loop is required")
	}

	return nil
}

// setSingleTurnEnv sets WEAVE_SINGLE_TURN=1 when singleTurn is true and
// returns a cleanup function that restores the previous env value.
func setSingleTurnEnv(singleTurn bool) func() {
	if !singleTurn {
		return func() {}
	}

	oldSingleTurn := os.Getenv("WEAVE_SINGLE_TURN")
	_ = os.Setenv("WEAVE_SINGLE_TURN", "1")

	return func() {
		if oldSingleTurn == "" {
			_ = os.Unsetenv("WEAVE_SINGLE_TURN")
		} else {
			_ = os.Setenv("WEAVE_SINGLE_TURN", oldSingleTurn)
		}
	}
}

func mergeCoreAndOptional(agentLoop string, optExts []string) []string {
	seen := make(map[string]bool)

	var result []string

	if !seen[agentLoop] {
		seen[agentLoop] = true
		result = append(result, agentLoop)
	}

	for _, name := range optExts {
		// The agent loop is handled as core, not optional.
		if name == agentLoop {
			continue
		}

		// When using a custom agent loop, skip the default "agent" to prevent
		// concurrent turn execution.
		if name == defaultAgentLoop && agentLoop != defaultAgentLoop {
			continue
		}

		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return result
}
