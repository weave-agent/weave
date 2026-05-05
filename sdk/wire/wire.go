package wire

//go:generate moq -fmt goimports -stub -skip-ensure -pkg wire -out wire_mock_test.go .. Bus Provider

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"weave/sdk"
)

type CoreWireConfig struct {
	AgentLoop  string
	Providers  []string
	SingleTurn bool
}

type Wired struct {
	extensions []sdk.Extension
	bus        sdk.Bus
}

func Wire(extNames []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
	if cfg == nil {
		cfg = noopConfig{}
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

	for _, ext := range exts {
		ext.Subscribe(bus)
	}

	return &Wired{extensions: exts, bus: bus}, nil
}

func WireWithCore(core CoreWireConfig, optExts []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
	if err := validateCore(core); err != nil {
		return nil, fmt.Errorf("wire: %w", err)
	}

	for _, p := range core.Providers {
		if !sdk.ProviderRegistered(p) {
			return nil, fmt.Errorf("wire: provider %q not registered", p)
		}
	}

	oldProvider := os.Getenv("WEAVE_PROVIDER")
	if oldProvider == "" {
		_ = os.Setenv("WEAVE_PROVIDER", core.Providers[0])

		defer func() {
			_ = os.Unsetenv("WEAVE_PROVIDER")
		}()
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

type noopConfig struct{}

func (noopConfig) FilePath() string                                { return "" }
func (noopConfig) ProviderConfig(string) *sdk.ProviderConfigEntry   { return nil }
func (noopConfig) ResolveKey(_, envVar string) (string, error)      { return os.Getenv(envVar), nil }
func (noopConfig) ToolConfig(string, any) error                     { return nil }
func (noopConfig) UIConfig(any) error                               { return nil }
func (noopConfig) IsHeadless() bool                                 { return true }
