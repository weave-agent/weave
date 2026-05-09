package sdk

import "sync"

//go:generate moq -fmt goimports -out sandbox_mock_test.go . Sandboxer

// Sandbox modes.
const (
	SandboxOff      = "off"
	SandboxReadonly = "readonly"
	SandboxAsk      = "ask"
	SandboxAuto     = "auto"
)

// SandboxModes is the ordered cycle of sandbox modes.
var SandboxModes = []string{SandboxOff, SandboxReadonly, SandboxAsk, SandboxAuto}

// Sandboxer wraps tool execution with OS-level sandboxing and path-based
// access policy. Extensions register a Sandboxer via SetSandboxer; tools
// query it via GetSandboxer (nil-safe).
type Sandboxer interface {
	// WrapCommand wraps a bash command string in an OS sandbox profile.
	// Returns the wrapped command or an error if the sandbox is unavailable.
	WrapCommand(cmd, dir string) (string, error)

	// AllowWrite reports whether the given path is allowed for write operations.
	AllowWrite(path string) bool

	// AllowRead reports whether the given path is allowed for read operations.
	AllowRead(path string) bool

	// Mode returns the current sandbox mode.
	Mode() string

	// SetMode changes the sandbox mode.
	SetMode(mode string)
}

var (
	sandboxerMu sync.RWMutex
	sandboxer   Sandboxer
)

// SetSandboxer registers the global Sandboxer instance.
func SetSandboxer(s Sandboxer) {
	sandboxerMu.Lock()
	sandboxer = s
	sandboxerMu.Unlock()
}

// GetSandboxer returns the global Sandboxer, or nil if none is registered.
func GetSandboxer() Sandboxer {
	sandboxerMu.RLock()
	defer sandboxerMu.RUnlock()

	return sandboxer
}

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
