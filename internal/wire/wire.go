package wire

//go:generate moq -fmt goimports -stub -skip-ensure -pkg wire -out wire_mock_test.go ../../sdk Bus Provider

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"

	"weave/internal/extmanage"
	"weave/internal/filemut"
	"weave/internal/filetracker"
	"weave/sdk"
	"weave/sdk/model"
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
}

func resolveExtensions(extNames []string, cfg sdk.Config) ([]sdk.Extension, error) {
	if cfg == nil {
		cfg = sdk.FilePathConfig("")
	}

	exts := make([]sdk.Extension, 0, len(extNames))

	for _, name := range extNames {
		ext, err := sdk.GetExtension(name, cfg)
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

		exts = append(exts, ext)
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

func WireExtensions(extNames []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
	exts, err := resolveExtensions(extNames, cfg)
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
	}

	if err := subscribeExtensions(exts, bus); err != nil {
		return nil, err
	}

	return &Wired{extensions: exts, bus: bus}, nil
}

func resolveSession(continueFlag bool, resumeID string, bus sdk.Bus) (string, []sdk.Message, error) {
	store := sdk.GetSessionStore()
	if store == nil {
		return "", nil, errors.New("no session store available")
	}

	sessionID, messages, err := resolveSessionFromStore(store, continueFlag, resumeID)
	if err != nil {
		return "", nil, err
	}

	if sessionID != "" {
		bus.Publish(sdk.NewEvent("session.resume", sdk.SessionResumePayload{
			SessionID: sessionID,
			Messages:  messages,
		}))
	}

	return sessionID, messages, nil
}

func resolveSessionFromStore(store sdk.SessionStore, continueFlag bool, resumeID string) (string, []sdk.Message, error) {
	if resumeID != "" {
		messages, err := store.LoadHistory(resumeID)
		if err != nil {
			return "", nil, fmt.Errorf("load session %s: %w", resumeID, err)
		}

		return resumeID, messages, nil
	}

	if continueFlag {
		return resolveContinueSession(store)
	}

	return "", nil, nil
}

func resolveContinueSession(store sdk.SessionStore) (string, []sdk.Message, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return "", nil, fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
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

func WireWithCore(core CoreWireConfig, optExts []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
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

	wired, err := WireExtensions(extNames, bus, cfg)
	if err != nil {
		return nil, err
	}

	if core.Continue || core.Resume != "" {
		if _, _, err := resolveSession(core.Continue, core.Resume, bus); err != nil {
			if cfg != nil && cfg.IsHeadless() {
				return nil, fmt.Errorf("session resume: %w", err)
			}

			slog.Warn("session resume failed", "error", err)
		}
	}

	go func() {
		bus.Publish(sdk.NewEvent("app.started", nil))

		if cfg == nil || !cfg.IsHeadless() {
			extmanage.FireUpdateCheck(bus)
		}
	}()

	return wired, nil
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
