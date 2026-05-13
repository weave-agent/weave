package wire

//go:generate moq -fmt goimports -stub -skip-ensure -pkg wire -out wire_mock_test.go .. Bus Provider

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"weave/internal/filemut"
	"weave/internal/filetracker"
	"weave/sdk"
)

const defaultAgentLoop = "loop"

type CoreWireConfig struct {
	AgentLoop  string
	SingleTurn bool
}

type Wired struct {
	extensions []sdk.Extension
	bus        sdk.Bus
}

func Wire(extNames []string, bus sdk.Bus, cfg sdk.Config) (*Wired, error) {
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

	for i, ext := range exts {
		if err := ext.Subscribe(bus); err != nil {
			for j := range slices.Backward(exts[:i]) {
				_ = exts[j].Close()
			}

			return nil, fmt.Errorf("wire: subscribe %s: %w", ext.Name(), err)
		}
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

	extNames := mergeCoreAndOptional(core.AgentLoop, optExts)

	for _, fn := range sdk.AppStartedHandlers() {
		handler := fn

		bus.On("app.started", func(e sdk.Event) error {
			if cfg != nil {
				handler(bus, cfg)
			}

			return nil
		})
	}

	wired, err := Wire(extNames, bus, cfg)
	if err != nil {
		return nil, err
	}

	go bus.Publish(sdk.NewEvent("app.started", nil))

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

		// When using a custom agent loop, skip the default "loop" to prevent
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
