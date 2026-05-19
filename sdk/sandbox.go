package sdk

// Sandbox event topics shared by sandbox-related extensions.
const (
	// SandboxRegisteredTopic publishes an sdk.Sandboxer payload.
	SandboxRegisteredTopic = "sandbox.registered"

	// SandboxModeChangeTopic publishes a string mode payload.
	SandboxModeChangeTopic = "sandbox.mode.change"

	// SandboxCycleTopic requests cycling to the next sandbox mode; payload is ignored.
	SandboxCycleTopic = "sandbox.cycle"
)

//go:generate moq -fmt goimports -out sandbox_mock_test.go . Sandboxer

// Sandboxer wraps tool execution with OS-level sandboxing and path-based
// access policy. Extensions register a Sandboxer by publishing a
// SandboxRegisteredTopic event with the Sandboxer as payload; tools and UI
// extensions receive it via bus subscription instead of calling GetSandboxer.
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
