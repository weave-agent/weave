package sdk

//go:generate moq -fmt goimports -out sandbox_mock_test.go . Sandboxer

import "context"

// Sandbox event topics shared by sandbox-related extensions.
const (
	// SandboxRegisteredTopic publishes an sdk.Sandboxer payload.
	SandboxRegisteredTopic = "sandbox.registered"

	// SandboxStatusRequestTopic requests an sdk.SandboxStatus payload.
	SandboxStatusRequestTopic = "sandbox.status.request"

	// SandboxStatusTopic publishes an sdk.SandboxStatus payload.
	SandboxStatusTopic = "sandbox.status"

	// SandboxExpansionRequestTopic publishes an sdk.SandboxExpansionRequest
	// payload when a sandboxed process needs broader containment boundaries.
	SandboxExpansionRequestTopic = "sandbox.expansion.request"

	// SandboxExpansionResolutionTopic publishes an
	// sdk.SandboxExpansionResolution payload after an expansion is resolved.
	SandboxExpansionResolutionTopic = "sandbox.expansion.resolution"
)

type SandboxAvailability string

const (
	SandboxAvailabilityAvailable   SandboxAvailability = "available"
	SandboxAvailabilityUnavailable SandboxAvailability = "unavailable"
	SandboxAvailabilityDegraded    SandboxAvailability = "degraded"
)

type SandboxExpansionState string

const (
	SandboxExpansionPending SandboxExpansionState = "pending"
	SandboxExpansionAllowed SandboxExpansionState = "allowed"
	SandboxExpansionDenied  SandboxExpansionState = "denied"
	SandboxExpansionExpired SandboxExpansionState = "expired"
)

type SandboxFilesystemAccess string

const (
	SandboxFilesystemRead      SandboxFilesystemAccess = "read"
	SandboxFilesystemWrite     SandboxFilesystemAccess = "write"
	SandboxFilesystemReadWrite SandboxFilesystemAccess = "read_write"
)

type SandboxNetworkAccess string

const (
	SandboxNetworkConnect SandboxNetworkAccess = "connect"
	SandboxNetworkListen  SandboxNetworkAccess = "listen"
)

// Sandboxer wraps approved command execution with OS-level containment.
// Extensions register a Sandboxer by publishing SandboxRegisteredTopic with
// the Sandboxer as payload.
type Sandboxer interface {
	WrapCommand(ctx context.Context, req SandboxCommandRequest) (SandboxCommand, error)
	Status(ctx context.Context) (SandboxStatus, error)
	RequestExpansion(ctx context.Context, req SandboxExpansionRequest) (SandboxExpansion, error)
	ResolveExpansion(ctx context.Context, expansionID string, resolution SandboxExpansionResolution) error
}

type SandboxCommandRequest struct {
	ID         string         `json:"id"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Command    string         `json:"command"`
	WorkingDir string         `json:"working_dir,omitempty"`
	Env        []string       `json:"env,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SandboxCommand struct {
	Command    string         `json:"command"`
	Args       []string       `json:"args,omitempty"`
	Env        []string       `json:"env,omitempty"`
	WorkingDir string         `json:"working_dir,omitempty"`
	Profile    string         `json:"profile,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SandboxStatus struct {
	Availability SandboxAvailability `json:"availability"`
	Provider     string              `json:"provider,omitempty"`
	Profile      string              `json:"profile,omitempty"`
	Reason       string              `json:"reason,omitempty"`
	Metadata     map[string]any      `json:"metadata,omitempty"`
}

type SandboxExpansionRequest struct {
	ID         string                       `json:"id"`
	ToolCallID string                       `json:"tool_call_id,omitempty"`
	Command    string                       `json:"command,omitempty"`
	WorkingDir string                       `json:"working_dir,omitempty"`
	Reason     string                       `json:"reason,omitempty"`
	Filesystem []SandboxFilesystemExpansion `json:"filesystem,omitempty"`
	Network    []SandboxNetworkExpansion    `json:"network,omitempty"`
	Metadata   map[string]any               `json:"metadata,omitempty"`
}

type SandboxFilesystemExpansion struct {
	Path   string                  `json:"path"`
	Access SandboxFilesystemAccess `json:"access"`
}

type SandboxNetworkExpansion struct {
	Host   string               `json:"host,omitempty"`
	Ports  []string             `json:"ports,omitempty"`
	Access SandboxNetworkAccess `json:"access"`
}

type SandboxExpansion struct {
	ID         string                      `json:"id"`
	RequestID  string                      `json:"request_id"`
	State      SandboxExpansionState       `json:"state"`
	Reason     string                      `json:"reason,omitempty"`
	Resolution *SandboxExpansionResolution `json:"resolution,omitempty"`
	Metadata   map[string]any              `json:"metadata,omitempty"`
}

type SandboxExpansionResolution struct {
	State     SandboxExpansionState `json:"state"`
	Reason    string                `json:"reason,omitempty"`
	ExpiresAt string                `json:"expires_at,omitempty"`
	Metadata  map[string]any        `json:"metadata,omitempty"`
}
