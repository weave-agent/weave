package sandbox

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/nniel-ape/gonfig"

	"weave/sdk"
)

// Sandbox modes.
const (
	ModeOff      = "off"
	ModeReadonly = "readonly"
	ModeAsk      = "ask"
	ModeAuto     = "auto"
)

// SandboxConfig holds user-configurable sandbox settings loaded via gonfig.
type SandboxConfig struct {
	Mode       string   `json:"mode" default:"auto" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable   []string `json:"writable" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite  []string `json:"deny_write" description:"Additional paths to block from writes"`
	DenyRead   []string `json:"deny_read" description:"Paths to block from reading"`
	Network    bool     `json:"network" default:"true" description:"Allow network access in sandbox"`
}

// config wraps SandboxConfig for gonfig loading.
type config struct {
	Sandbox SandboxConfig
}

// Sandbox implements sdk.Sandboxer with configurable modes and path policies.
type Sandbox struct {
	cfg SandboxConfig
	mu  sync.RWMutex
}

func init() {
	sdk.RegisterExtension("sandbox", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewSandbox(cfg)
	})
}

// NewSandbox creates a new Sandbox extension, loading config via gonfig.
func NewSandbox(cfg sdk.Config) (*Sandbox, error) {
	var c config

	opts := []gonfig.Option{gonfig.WithEnvPrefix("WEAVE")}
	if cfg != nil && cfg.FilePath() != "" {
		opts = append(opts, gonfig.WithFile(cfg.FilePath()))
	}

	if err := gonfig.Load(&c, opts...); err != nil {
		return nil, fmt.Errorf("sandbox config: %w", err)
	}

	sc := c.Sandbox
	if sc.Mode == "" {
		sc.Mode = ModeAuto
	}

	s := &Sandbox{cfg: sc}
	sdk.SetSandboxer(s)
	return s, nil
}

func (s *Sandbox) Name() string { return "sandbox" }

func (s *Sandbox) Subscribe(bus sdk.Bus) error {
	bus.On("sandbox.mode.change", func(ev sdk.Event) error {
		mode, ok := ev.Payload.(string)
		if !ok {
			slog.Warn("sandbox: invalid mode.change payload", "payload", ev.Payload)
			return nil
		}

		s.mu.Lock()
		s.cfg.Mode = mode
		s.mu.Unlock()

		slog.Info("sandbox: mode changed", "mode", mode)
		return nil
	})

	return nil
}

func (s *Sandbox) Close() error {
	return nil
}

// Mode returns the current sandbox mode (thread-safe).
func (s *Sandbox) Mode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Mode
}

// WrapCommand wraps a bash command in an OS sandbox profile.
// Behavior depends on the active mode.
func (s *Sandbox) WrapCommand(cmd, dir string) (string, error) {
	s.mu.RLock()
	mode := s.cfg.Mode
	s.mu.RUnlock()

	switch mode {
	case ModeOff:
		return cmd, nil
	case ModeAuto:
		return wrapCommandPlatform(cmd, dir)
	default:
		return cmd, nil
	}
}

// AllowWrite reports whether the given path is allowed for write operations.
func (s *Sandbox) AllowWrite(path string) bool {
	s.mu.RLock()
	mode := s.cfg.Mode
	s.mu.RUnlock()

	if mode == ModeOff {
		return true
	}

	if isDeniedWrite(path) {
		return false
	}

	for _, deny := range s.cfg.DenyWrite {
		if pathMatches(path, deny) {
			return false
		}
	}

	if len(s.cfg.Writable) == 0 {
		return true
	}

	for _, w := range s.cfg.Writable {
		if pathMatches(path, w) {
			return true
		}
	}

	return false
}

// AllowRead reports whether the given path is allowed for read operations.
func (s *Sandbox) AllowRead(path string) bool {
	s.mu.RLock()
	mode := s.cfg.Mode
	s.mu.RUnlock()

	if mode == ModeOff {
		return true
	}

	if isDeniedRead(path) {
		return false
	}

	for _, deny := range s.cfg.DenyRead {
		if pathMatches(path, deny) {
			return false
		}
	}

	return true
}

// SetMode changes the sandbox mode (for testing and bus events).
func (s *Sandbox) SetMode(mode string) {
	s.mu.Lock()
	s.cfg.Mode = mode
	s.mu.Unlock()
}

// getCurrentSandbox returns the global Sandboxer as a *Sandbox, or nil.
func getCurrentSandbox() *Sandbox {
	sb := sdk.GetSandboxer()
	if sb == nil {
		return nil
	}
	if s, ok := sb.(*Sandbox); ok {
		return s
	}
	return nil
}
