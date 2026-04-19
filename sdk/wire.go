package sdk

import (
	"errors"
	"fmt"
)

type CoreWireConfig struct {
	AgentLoop string
	Providers []string
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
			for i := len(exts) - 1; i >= 0; i-- {
				_ = exts[i].Close()
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

	merged := mergeCoreAndOptional(core, optExts)

	return Wire(merged, bus, cfg)
}

func (w *Wired) Close() error {
	var errs []error

	for i := len(w.extensions) - 1; i >= 0; i-- {
		if err := w.extensions[i].Close(); err != nil {
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
