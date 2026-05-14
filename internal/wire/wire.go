package wire

//go:generate moq -fmt goimports -stub -skip-ensure -pkg wire -out wire_mock_test.go ../../sdk Bus Provider

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"

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
			if sdk.ToolRegistered(name) || sdk.ProviderRegistered(name) || sdk.UIExtensionRegistered(name) {
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
			log.Printf("weave: check auth for %s: %v", name, err)
		}

		model.SetProviderAuth(name, hasAuth)
	}

	if err := subscribeExtensions(exts, bus); err != nil {
		return nil, err
	}

	return &Wired{extensions: exts, bus: bus}, nil
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
