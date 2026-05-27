package sdk

//go:generate moq -fmt goimports -stub -out guardian_mock_test.go . Guardian

import "context"

// Guardian event topics shared by guardian-related extensions.
const (
	// GuardianRegisteredTopic publishes an sdk.Guardian payload.
	GuardianRegisteredTopic = "guardian.registered"

	// GuardianDecisionTopic publishes an sdk.GuardianDecision payload after a
	// request has been evaluated.
	GuardianDecisionTopic = "guardian.decision"

	// GuardianApprovalRequestTopic publishes an sdk.GuardianApprovalRequest
	// payload when a decision requires user approval.
	GuardianApprovalRequestTopic = "guardian.approval.request"

	// GuardianApprovalResolutionTopic publishes an sdk.GuardianApprovalResolution
	// payload after a pending approval is resolved.
	GuardianApprovalResolutionTopic = "guardian.approval.resolution"

	// GuardianProfileChangeTopic publishes an sdk.GuardianProfileChange payload.
	GuardianProfileChangeTopic = "guardian.profile.change"

	// GuardianPolicyOverlayPushTopic publishes an sdk.GuardianPolicyOverlay
	// payload to add or replace a runtime policy overlay.
	GuardianPolicyOverlayPushTopic = "guardian.policy.overlay.push"

	// GuardianPolicyOverlayPopTopic publishes an sdk.GuardianPolicyOverlayPop
	// payload to remove a runtime policy overlay.
	GuardianPolicyOverlayPopTopic = "guardian.policy.overlay.pop"

	// GuardianSnapshotRequestTopic requests an sdk.GuardianSnapshot payload.
	GuardianSnapshotRequestTopic = "guardian.snapshot.request"

	// GuardianSnapshotTopic publishes an sdk.GuardianSnapshot payload.
	GuardianSnapshotTopic = "guardian.snapshot"

	// GuardianClearGrantsTopic publishes an sdk.GuardianClearGrantsRequest
	// payload, or nil to clear all active grants.
	GuardianClearGrantsTopic = "guardian.grants.clear"
)

type GuardianAction string

const (
	GuardianActionRead    GuardianAction = "read"
	GuardianActionWrite   GuardianAction = "write"
	GuardianActionDelete  GuardianAction = "delete"
	GuardianActionExec    GuardianAction = "exec"
	GuardianActionNetwork GuardianAction = "network"
	GuardianActionUnknown GuardianAction = "unknown"
)

type GuardianDecisionAction string

const (
	GuardianDecisionAllow GuardianDecisionAction = "allow"
	GuardianDecisionAsk   GuardianDecisionAction = "ask"
	GuardianDecisionBlock GuardianDecisionAction = "block"
)

type GuardianResolutionAction string

const (
	GuardianResolutionAllow GuardianResolutionAction = "allow"
	GuardianResolutionDeny  GuardianResolutionAction = "deny"
)

type GuardianGrantScope string

const (
	GuardianGrantScopeOnce    GuardianGrantScope = "once"
	GuardianGrantScopeSession GuardianGrantScope = "session"
	GuardianGrantScopeProfile GuardianGrantScope = "profile"
)

// Guardian decides whether requested tool actions may run. Extensions register
// a Guardian by publishing GuardianRegisteredTopic with the Guardian as payload.
type Guardian interface {
	Decide(ctx context.Context, req GuardianRequest) (GuardianDecision, error)
	Resolve(ctx context.Context, decisionID string, resolution GuardianResolution) error
	Snapshot(ctx context.Context) (GuardianSnapshot, error)
}

type GuardianRequest struct {
	ID          string         `json:"id"`
	ToolCallID  string         `json:"tool_call_id,omitempty"`
	ToolName    string         `json:"tool_name"`
	Action      GuardianAction `json:"action"`
	Command     string         `json:"command,omitempty"`
	Path        string         `json:"path,omitempty"`
	WorkingDir  string         `json:"working_dir,omitempty"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type GuardianDecision struct {
	ID             string                 `json:"id"`
	RequestID      string                 `json:"request_id"`
	Action         GuardianDecisionAction `json:"action"`
	Reason         string                 `json:"reason,omitempty"`
	Profile        string                 `json:"profile,omitempty"`
	Approval       *GuardianApproval      `json:"approval,omitempty"`
	MatchedGrantID string                 `json:"matched_grant_id,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
}

type GuardianApproval struct {
	ID            string               `json:"id"`
	DecisionID    string               `json:"decision_id"`
	Request       GuardianRequest      `json:"request"`
	AllowedScopes []GuardianGrantScope `json:"allowed_scopes,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

type GuardianApprovalRequest struct {
	Approval GuardianApproval `json:"approval"`
}

type GuardianResolution struct {
	Action GuardianResolutionAction `json:"action"`
	Scope  GuardianGrantScope       `json:"scope,omitempty"`
	Reason string                   `json:"reason,omitempty"`
}

type GuardianApprovalResolution struct {
	ApprovalID string             `json:"approval_id"`
	DecisionID string             `json:"decision_id"`
	Resolution GuardianResolution `json:"resolution"`
}

type GuardianProfile struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Rules       []GuardianProfileRule `json:"rules,omitempty"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
}

type GuardianProfileRule struct {
	Actions  []GuardianAction       `json:"actions,omitempty"`
	Decision GuardianDecisionAction `json:"decision"`
	Reason   string                 `json:"reason,omitempty"`
	Metadata map[string]any         `json:"metadata,omitempty"`
}

type GuardianPolicyOverlay struct {
	ID                 string                `json:"id"`
	Source             string                `json:"source,omitempty"`
	Description        string                `json:"description,omitempty"`
	Rules              []GuardianProfileRule `json:"rules,omitempty"`
	OverrideHardBlocks bool                  `json:"override_hard_blocks,omitempty"`
}

type GuardianPolicyOverlayPop struct {
	ID     string `json:"id"`
	Source string `json:"source,omitempty"`
}

type GuardianProfileChange struct {
	PreviousProfile string `json:"previous_profile,omitempty"`
	CurrentProfile  string `json:"current_profile"`
}

type GuardianGrant struct {
	ID         string             `json:"id"`
	Scope      GuardianGrantScope `json:"scope"`
	Request    GuardianRequest    `json:"request"`
	Resolution GuardianResolution `json:"resolution"`
	CreatedAt  string             `json:"created_at,omitempty"`
	ExpiresAt  string             `json:"expires_at,omitempty"`
}

type GuardianSnapshot struct {
	CurrentProfile string                     `json:"current_profile"`
	Profiles       map[string]GuardianProfile `json:"profiles,omitempty"`
	Grants         []GuardianGrant            `json:"grants,omitempty"`
	Pending        []GuardianApproval         `json:"pending,omitempty"`
}

type GuardianClearGrantsRequest struct {
	GrantIDs []string `json:"grant_ids,omitempty"`
	Scope    string   `json:"scope,omitempty"`
}
