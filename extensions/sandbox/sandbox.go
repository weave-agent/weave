package sandbox

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nniel-ape/gonfig"

	"weave/sdk"
)

// SandboxConfig holds user-configurable sandbox settings loaded via gonfig.
type SandboxConfig struct {
	Mode      string   `json:"mode" default:"auto" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable  []string `json:"writable" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite []string `json:"deny_write" description:"Additional paths to block from writes"`
	DenyRead  []string `json:"deny_read" description:"Paths to block from reading"`
	Network   bool     `json:"network" default:"true" description:"Allow network access in sandbox"`
}

const keyCommand = "command"

// config wraps SandboxConfig for gonfig loading.
type config struct {
	Sandbox SandboxConfig
}

// Sandbox implements sdk.Sandboxer with configurable modes and path policies.
type Sandbox struct {
	cfg       SandboxConfig
	bus       sdk.Bus
	headless  bool
	cwd       string
	pending   []*askPending
	allowlist []string
	closed    bool
	mu        sync.RWMutex
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
		sc.Mode = sdk.SandboxAuto
	}

	headless := cfg == nil || cfg.IsHeadless()
	cwd, _ := os.Getwd()
	s := &Sandbox{cfg: sc, headless: headless, cwd: cwd}
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

		if !isValidMode(mode) {
			slog.Warn("sandbox: invalid mode", "mode", mode)
			return nil
		}

		s.mu.Lock()
		s.cfg.Mode = mode

		if mode != sdk.SandboxAsk {
			for _, p := range s.pending {
				select {
				case p.result <- false:
				default:
				}
			}

			s.pending = s.pending[:0]
		}
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

	bus.On("sandbox.trust", func(ev sdk.Event) error {
		payload, ok := ev.Payload.(map[string]string)
		if !ok {
			return nil
		}

		pattern := payload["pattern"]
		if pattern == "" {
			return nil
		}

		s.mu.Lock()
		s.allowlist = append(s.allowlist, pattern)
		s.mu.Unlock()

		slog.Info("sandbox: trusted pattern for session", "pattern", pattern)

		return nil
	})

	return nil
}

// resolvePending resolves the pending ask-mode command matching the event's command.
func (s *Sandbox) resolvePending(ev sdk.Event, approved bool) error {
	payload, ok := ev.Payload.(map[string]string)
	if !ok {
		return nil
	}

	cmd := payload[keyCommand]

	s.mu.Lock()
	for i, p := range s.pending {
		if p.cmd == cmd {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			s.mu.Unlock()

			p.result <- approved

			return nil
		}
	}
	s.mu.Unlock()

	return nil
}

func (s *Sandbox) Close() error {
	s.mu.Lock()
	s.closed = true

	for _, p := range s.pending {
		select {
		case p.result <- false:
		default:
		}
	}

	s.pending = nil
	s.mu.Unlock()

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
	cfg := s.cfg
	s.mu.RUnlock()

	switch cfg.Mode {
	case sdk.SandboxOff:
		return cmd, nil
	case sdk.SandboxAuto:
		return wrapCommandPlatformWithConfig(cmd, dir, cfg)
	case sdk.SandboxReadonly:
		return s.wrapCommandReadonly(cmd, dir)
	case sdk.SandboxAsk:
		return s.wrapCommandAsk(cmd)
	default:
		return cmd, nil
	}
}

// AllowWrite reports whether the given path is allowed for write operations.
func (s *Sandbox) AllowWrite(path string) bool {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.Mode == sdk.SandboxOff {
		return true
	}

	if cfg.Mode == sdk.SandboxReadonly {
		return false
	}

	if isDeniedWrite(path) {
		return false
	}

	for _, deny := range cfg.DenyWrite {
		if pathMatches(path, deny) {
			return false
		}
	}

	if len(cfg.Writable) == 0 {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}

		if s.cwd != "" {
			return abs == s.cwd || strings.HasPrefix(abs, s.cwd+"/")
		}

		return false
	}

	for _, w := range cfg.Writable {
		if pathMatches(path, w) {
			return true
		}
	}

	return false
}

// AllowRead reports whether the given path is allowed for read operations.
func (s *Sandbox) AllowRead(path string) bool {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.Mode == sdk.SandboxOff {
		return true
	}

	if isDeniedRead(path) {
		return false
	}

	for _, deny := range cfg.DenyRead {
		if pathMatches(path, deny) {
			return false
		}
	}

	return true
}

// wrapCommandReadonly wraps a command with no writable paths in the sandbox profile.
func (s *Sandbox) wrapCommandReadonly(cmd, dir string) (string, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

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

	// Check session allowlist before prompting.
	s.mu.RLock()

	trimmed := strings.TrimSpace(cmd)
	for _, pattern := range s.allowlist {
		if trimmed == pattern || strings.HasPrefix(trimmed, pattern+" ") {
			s.mu.RUnlock()

			return cmd, nil
		}
	}

	closed := s.closed
	s.mu.RUnlock()

	if closed {
		return "", errors.New("sandbox: extension closed")
	}

	result := make(chan bool, 1)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", errors.New("sandbox: extension closed")
	}

	s.pending = append(s.pending, &askPending{cmd: cmd, result: result})
	s.mu.Unlock()

	s.bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{"command": cmd}))

	select {
	case approved := <-result:
		if !approved {
			return "", fmt.Errorf("sandbox: command denied: %s", cmd)
		}

		return cmd, nil
	case <-time.After(5 * time.Minute):
		s.mu.Lock()
		for i, p := range s.pending {
			if p.cmd == cmd {
				s.pending = append(s.pending[:i], s.pending[i+1:]...)
				break
			}
		}
		s.mu.Unlock()

		return "", fmt.Errorf("sandbox: command approval timed out: %s", cmd)
	}
}

type askPending struct {
	cmd    string
	result chan bool
}

// SetMode changes the sandbox mode (for testing and bus events).
func (s *Sandbox) SetMode(mode string) {
	if !isValidMode(mode) {
		return
	}

	s.mu.Lock()
	s.cfg.Mode = mode
	s.mu.Unlock()
}

func isValidMode(mode string) bool {
	switch mode {
	case sdk.SandboxOff, sdk.SandboxReadonly, sdk.SandboxAsk, sdk.SandboxAuto:
		return true
	default:
		return false
	}
}
