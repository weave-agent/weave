package sdk

import (
	"errors"
	"fmt"
	"os"
	"slices"
)

type CoreWireConfig struct {
	AgentLoop  string
	Providers  []string
	SingleTurn bool
}

// Wired holds all extensions wired to a bus. Call Close to shut down.
type Wired struct {
	extensions []Extension
	bus        Bus
}

func Wire(extNames []string, bus Bus, cfg Config) (*Wired, error) {
	cfg = configOrDefault(cfg)
	exts := make([]Extension, 0, len(extNames))

	for _, name := range extNames {
		ext, err := GetExtension(name, cfg)
		if err != nil {
			// Tools and providers are registered via blank import but resolved
			// through their own registries at runtime, not wired as extensions.
			if ToolRegistered(name) || ProviderRegistered(name) {
				continue
			}

			for _, v := range slices.Backward(exts) {
				_ = v.Close()
			}

			return nil, fmt.Errorf("wire: %w", err)
		}

		exts = append(exts, ext)
	}

	for _, ext := range exts {
		ext.Subscribe(bus)
	}

	return &Wired{extensions: exts, bus: bus}, nil
}

func WireWithCore(core CoreWireConfig, optExts []string, bus Bus, cfg Config) (*Wired, error) {
	if err := validateCore(core); err != nil {
		return nil, fmt.Errorf("wire: %w", err)
	}

	// Validate that provider factories exist (they are resolved on demand by
	// the agent loop, not wired as extensions).
	for _, p := range core.Providers {
		if !ProviderRegistered(p) {
			return nil, fmt.Errorf("wire: provider %q not registered", p)
		}
	}

	// Sync the primary provider to the env var that the loop extension reads
	// in its factory, so direct WireWithCore callers don't need to set it.
	if os.Getenv("WEAVE_PROVIDER") == "" {
		_ = os.Setenv("WEAVE_PROVIDER", core.Providers[0])
	}

	if core.SingleTurn {
		oldSingleTurn := os.Getenv("WEAVE_SINGLE_TURN")
		_ = os.Setenv("WEAVE_SINGLE_TURN", "1")

		defer func() {
			if oldSingleTurn == "" {
				_ = os.Unsetenv("WEAVE_SINGLE_TURN")
			} else {
				_ = os.Setenv("WEAVE_SINGLE_TURN", oldSingleTurn)
			}
		}()
	}

	// Only wire the agent-loop extension and optional extensions.
	extNames := mergeCoreAndOptional(CoreWireConfig{AgentLoop: core.AgentLoop}, optExts)

	return Wire(extNames, bus, cfg)
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

	if len(core.Providers) == 0 {
		return errors.New("at least one provider is required")
	}

	seen := make(map[string]bool, len(core.Providers))
	for _, p := range core.Providers {
		if seen[p] {
			return fmt.Errorf("duplicate provider %q", p)
		}

		seen[p] = true
	}

	return nil
}

func mergeCoreAndOptional(core CoreWireConfig, optExts []string) []string {
	seen := make(map[string]bool)

	var result []string

	for _, name := range core.Providers {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	if !seen[core.AgentLoop] {
		seen[core.AgentLoop] = true
		result = append(result, core.AgentLoop)
	}

	for _, name := range optExts {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return result
}
