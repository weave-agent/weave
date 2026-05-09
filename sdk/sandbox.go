package sdk

//go:generate moq -fmt goimports -out sandbox_mock_test.go . Sandboxer

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
}

var sandboxer Sandboxer

// SetSandboxer registers the global Sandboxer instance.
func SetSandboxer(s Sandboxer) {
	sandboxer = s
}

// GetSandboxer returns the global Sandboxer, or nil if none is registered.
func GetSandboxer() Sandboxer {
	return sandboxer
}
