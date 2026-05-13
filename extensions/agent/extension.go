package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"weave/sdk"
)

// AgentExtension owns the entire conversation lifecycle:
// prompt assembly, turn loop, tool execution, skill discovery, and context file loading.
type AgentExtension struct {
	cfg    sdk.Config
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func init() {
	sdk.RegisterExtension("agent", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
		return NewAgentExtension(cfg)
	})
}

func NewAgentExtension(cfg sdk.Config) (*AgentExtension, error) {
	return &AgentExtension{cfg: cfg}, nil
}

func (a *AgentExtension) Name() string { return "agent" }

func (a *AgentExtension) Subscribe(bus sdk.Bus) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		return errors.New("agent: Subscribe called twice without Close")
	}

	ctx, cancel := context.WithCancel(context.Background())

	a.cancel = cancel
	a.done = make(chan struct{})

	go a.run(ctx, bus)

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

func (a *AgentExtension) run(ctx context.Context, bus sdk.Bus) {
	defer close(a.done)

	// TODO: port turn loop, skill discovery, and context loading in later tasks.
	_ = ctx
	_ = bus
}

// projectDir returns the project directory from config, or derives it from the
// config file path.
func (a *AgentExtension) projectDir() string {
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
