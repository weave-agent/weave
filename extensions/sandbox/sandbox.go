package sandbox

import (
	"errors"
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
	Mode      string   `json:"mode" default:"auto" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable  []string `json:"writable" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite []string `json:"deny_write" description:"Additional paths to block from writes"`
	DenyRead  []string `json:"deny_read" description:"Paths to block from reading"`
	Network   bool     `json:"network" default:"true" description:"Allow network access in sandbox"`
}

// config wraps SandboxConfig for gonfig loading.
type config struct {
	Sandbox SandboxConfig
}

// Sandbox implements sdk.Sandboxer with configurable modes and path policies.
type Sandbox struct {
	cfg      SandboxConfig
	bus      sdk.Bus
	headless bool
	pending  []*askPending
	mu       sync.RWMutex
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

	headless := cfg == nil || cfg.IsHeadless()
	s := &Sandbox{cfg: sc, headless: headless}
	sdk.SetSandboxer(s)

	return s, nil
}

func (s *Sandbox) Name() string { return "sandbox" }

func (s *Sandbox) Subscribe(bus sdk.Bus) error {
	s.bus = bus

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

	// Handle approval responses from TUI ask dialog.
	bus.On("sandbox.approved", func(ev sdk.Event) error {
		return s.resolvePending(ev, true)
	})

	bus.On("sandbox.denied", func(ev sdk.Event) error {
		return s.resolvePending(ev, false)
	})

	return nil
}

// resolvePending resolves the oldest pending ask-mode command.
func (s *Sandbox) resolvePending(_ sdk.Event, approved bool) error {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return nil
	}

	p := s.pending[0]
	s.pending = s.pending[1:]
	s.mu.Unlock()

	p.result <- askResult{approved: approved}

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
	case ModeReadonly:
		return wrapCommandReadonly(cmd, dir)
	case ModeAsk:
		return s.wrapCommandAsk(cmd)
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

	if mode == ModeReadonly {
		return false
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

// wrapCommandReadonly wraps a command with no writable paths in the sandbox profile.
func wrapCommandReadonly(cmd, dir string) (string, error) {
	s := getCurrentSandbox()

	var cfg SandboxConfig

	if s != nil {
		s.mu.RLock()
		cfg = s.cfg
		s.mu.RUnlock()
	}

	cfg.Writable = nil

	return wrapCommandPlatformWithConfig(cmd, dir, cfg)
}

// wrapCommandAsk publishes an approval request on the bus and waits for a response.
func (s *Sandbox) wrapCommandAsk(cmd string) (string, error) {
	if s.headless {
		return "", fmt.Errorf("command requires approval (headless mode): %s", cmd)
	}

	if s.bus == nil {
		return "", errors.New("sandbox: bus not available for ask mode")
	}

	result := make(chan askResult, 1)

	s.mu.Lock()
	s.pending = append(s.pending, &askPending{cmd: cmd, result: result})
	s.mu.Unlock()

	s.bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{"command": cmd}))

	r := <-result
	if !r.approved {
		return "", fmt.Errorf("sandbox: command denied: %s", cmd)
	}

	return cmd, nil
}

type askResult struct {
	approved bool
}

type askPending struct {
	cmd    string
	result chan askResult
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
