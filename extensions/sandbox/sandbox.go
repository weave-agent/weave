package sandbox

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"weave/sdk"
)

// Sandbox modes.
const (
	SandboxOff      = "off"
	SandboxReadonly = "readonly"
	SandboxAsk      = "ask"
	SandboxAuto     = "auto"
)

// SandboxModes is the ordered cycle of sandbox modes.
var SandboxModes = []string{SandboxOff, SandboxReadonly, SandboxAsk, SandboxAuto}

// NextSandboxMode returns the next mode in the cycle order.
func NextSandboxMode(current string) string {
	for i, m := range SandboxModes {
		if m == current {
			if i+1 < len(SandboxModes) {
				return SandboxModes[i+1]
			}

			return SandboxModes[0]
		}
	}

	return SandboxModes[0]
}

// SandboxConfig holds user-configurable sandbox settings loaded via gonfig.
type SandboxConfig struct {
	Mode      string   `json:"mode" default:"auto" env:"MODE" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable  []string `json:"writable" env:"WRITABLE" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite []string `json:"deny_write" env:"DENY_WRITE" description:"Additional paths to block from writes"`
	DenyRead  []string `json:"deny_read" env:"DENY_READ" description:"Paths to block from reading"`
	Network   bool     `json:"network" default:"true" env:"NETWORK" description:"Allow network access in sandbox"`
}

const keyCommand = "command"

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
	sdk.RegisterExtensionWithScope[SandboxConfig]("sandbox", "sandbox", func(cfg sdk.Config, sc SandboxConfig) (sdk.Extension, error) {
		return NewSandbox(cfg, sc)
	})
}

// NewSandbox creates a new Sandbox extension with the given config.
func NewSandbox(cfg sdk.Config, sc SandboxConfig) (*Sandbox, error) {
	if sc.Mode == "" {
		sc.Mode = SandboxAuto
	} else if !isValidMode(sc.Mode) {
		slog.Warn("sandbox: invalid mode in config, falling back to auto", "mode", sc.Mode)
		sc.Mode = SandboxAuto
	}

	headless := cfg == nil || cfg.IsHeadless()
	cwd := resolveAbsUnsafe()

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

		if mode != SandboxAsk {
			for _, p := range s.pending {
				select {
				case p.result <- false:
				default:
				}
			}

			s.pending = s.pending[:0]
		}
		s.mu.Unlock()

		slog.Debug("sandbox: mode changed", "mode", mode)

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

		slog.Debug("sandbox: trusted pattern for session", "pattern", pattern)

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

	sdk.SetSandboxer(nil)

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
	case SandboxOff:
		return cmd, nil
	case SandboxAuto:
		return wrapCommandPlatformWithConfig(cmd, dir, cfg)
	case SandboxReadonly:
		return s.wrapCommandReadonly(cmd, dir)
	case SandboxAsk:
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

	if cfg.Mode == SandboxOff {
		return true
	}

	if cfg.Mode == SandboxReadonly {
		return false
	}

	abs := resolveAbs(path)

	// Deny if path could not be resolved — prevents bypass via relative paths.
	if !filepath.IsAbs(abs) {
		return false
	}

	if isDeniedWrite(abs, s.cwd) {
		return false
	}

	// User-configured deny rules are enforced in all modes (hard policy).
	for _, deny := range cfg.DenyWrite {
		if pathMatches(abs, deny, s.cwd) {
			return false
		}
	}

	// Ask mode: enforce writable-path policy, then prompt in interactive.
	if cfg.Mode == SandboxAsk {
		if s.headless {
			return false
		}

		if !isWritablePath(abs, cfg.Writable, s.cwd) {
			return false
		}

		return s.promptFileAccess(abs, "write")
	}

	if !isWritablePath(abs, cfg.Writable, s.cwd) {
		return false
	}

	return true
}

// isWritablePath checks whether abs is within a configured writable zone.
func isWritablePath(abs string, writable []string, cwd string) bool {
	if len(writable) == 0 {
		if cwd != "" {
			return abs == cwd || strings.HasPrefix(abs, cwd+"/")
		}

		return false
	}

	for _, w := range writable {
		if pathMatches(abs, w, cwd) {
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

	if cfg.Mode == SandboxOff {
		return true
	}

	abs := resolveAbs(path)

	// If path could not be resolved to absolute, deny to prevent
	// mandatory deny rules from being bypassed by relative paths.
	if !filepath.IsAbs(abs) {
		return false
	}

	if isDeniedRead(abs) {
		return false
	}

	for _, deny := range cfg.DenyRead {
		if pathMatches(abs, deny, s.cwd) {
			return false
		}
	}

	return true
}

// noWritable is a sentinel indicating the sandbox should have zero write paths.
// nil/empty Writable means "use default (CWD)", while []string{""} means "no writes".
var noWritable = []string{""}

// wrapCommandReadonly wraps a command with no writable paths in the sandbox profile.
func (s *Sandbox) wrapCommandReadonly(cmd, dir string) (string, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	cfg.Writable = noWritable

	return wrapCommandPlatformWithConfig(cmd, dir, cfg)
}

// promptFileAccess asks the user to approve a file operation via the TUI dialog.
func (s *Sandbox) promptFileAccess(path, op string) bool {
	if s.bus == nil {
		return false
	}

	label := op + ": " + path

	// Check session allowlist before prompting.
	s.mu.RLock()

	for _, pattern := range s.allowlist {
		if label == pattern || strings.HasPrefix(label, pattern+" ") {
			s.mu.RUnlock()

			return true
		}
	}

	closed := s.closed
	s.mu.RUnlock()

	if closed {
		return false
	}

	result := make(chan bool, 1)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}

	s.pending = append(s.pending, &askPending{cmd: label, result: result})
	s.mu.Unlock()

	s.bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{"command": label}))

	select {
	case approved := <-result:
		return approved
	case <-time.After(5 * time.Minute):
		s.mu.Lock()
		for i, p := range s.pending {
			if p.cmd == label {
				s.pending = append(s.pending[:i], s.pending[i+1:]...)
				break
			}
		}
		s.mu.Unlock()

		return false
	}
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
// When switching away from ask mode, pending requests are canceled.
func (s *Sandbox) SetMode(mode string) {
	if !isValidMode(mode) {
		return
	}

	s.mu.Lock()
	s.cfg.Mode = mode

	if mode != SandboxAsk {
		for _, p := range s.pending {
			select {
			case p.result <- false:
			default:
			}
		}

		s.pending = s.pending[:0]
	}

	s.mu.Unlock()
}

func isValidMode(mode string) bool {
	switch mode {
	case SandboxOff, SandboxReadonly, SandboxAsk, SandboxAuto:
		return true
	default:
		return false
	}
}
