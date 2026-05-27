package sdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardianMock_DecideResolveSnapshot(t *testing.T) {
	ctx := context.Background()
	req := GuardianRequest{
		ID:       "req-1",
		ToolName: "bash",
		Action:   GuardianActionExec,
		Command:  "go test ./sdk/...",
	}
	resolution := GuardianResolution{
		Action: GuardianResolutionAllow,
		Scope:  GuardianGrantScopeOnce,
	}

	mock := &GuardianMock{
		DecideFunc: func(context.Context, GuardianRequest) (GuardianDecision, error) {
			return GuardianDecision{
				ID:        "decision-1",
				RequestID: "req-1",
				Action:    GuardianDecisionAsk,
				Approval: &GuardianApproval{
					ID:         "approval-1",
					DecisionID: "decision-1",
					Request:    req,
				},
			}, nil
		},
		ResolveFunc: func(context.Context, string, GuardianResolution) error {
			return nil
		},
		SnapshotFunc: func(context.Context) (GuardianSnapshot, error) {
			return GuardianSnapshot{
				CurrentProfile: "auto",
				Grants: []GuardianGrant{
					{
						ID:         "grant-1",
						Scope:      GuardianGrantScopeOnce,
						Request:    req,
						Resolution: resolution,
					},
				},
			}, nil
		},
	}

	decision, err := mock.Decide(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, decision.Approval)
	assert.Equal(t, GuardianDecisionAsk, decision.Action)
	assert.Equal(t, "approval-1", decision.Approval.ID)

	require.NoError(t, mock.Resolve(ctx, decision.ID, resolution))

	snapshot, err := mock.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, "auto", snapshot.CurrentProfile)
	require.Len(t, snapshot.Grants, 1)
	assert.Equal(t, "grant-1", snapshot.Grants[0].ID)
}

func TestGuardianEventTopics(t *testing.T) {
	assert.Equal(t, "guardian.registered", GuardianRegisteredTopic)
	assert.Equal(t, "guardian.decision", GuardianDecisionTopic)
	assert.Equal(t, "guardian.approval.request", GuardianApprovalRequestTopic)
	assert.Equal(t, "guardian.approval.resolution", GuardianApprovalResolutionTopic)
	assert.Equal(t, "guardian.profile.change", GuardianProfileChangeTopic)
	assert.Equal(t, "guardian.policy.overlay.push", GuardianPolicyOverlayPushTopic)
	assert.Equal(t, "guardian.policy.overlay.pop", GuardianPolicyOverlayPopTopic)
	assert.Equal(t, "guardian.snapshot.request", GuardianSnapshotRequestTopic)
	assert.Equal(t, "guardian.snapshot", GuardianSnapshotTopic)
	assert.Equal(t, "guardian.grants.clear", GuardianClearGrantsTopic)
}

func TestGuardianPayloadsJSONRoundTrip(t *testing.T) {
	payload := GuardianSnapshot{
		CurrentProfile: "team",
		Profiles: map[string]GuardianProfile{
			"team": {
				Name:        "team",
				Description: "team defaults",
				Rules: []GuardianProfileRule{
					{
						Actions:  []GuardianAction{GuardianActionWrite, GuardianActionNetwork},
						Decision: GuardianDecisionAsk,
						Reason:   "requires approval",
					},
				},
			},
		},
		Overlays: []GuardianPolicyOverlay{
			{
				ID:          "overlay-1",
				Source:      "plan-mode",
				Description: "temporary plan policy",
				Rules: []GuardianProfileRule{
					{
						Actions:  []GuardianAction{GuardianActionWrite},
						Decision: GuardianDecisionAllow,
						Reason:   "trusted plan edits",
					},
				},
				OverrideHardBlocks: true,
			},
		},
		Grants: []GuardianGrant{
			{
				ID:    "grant-1",
				Scope: GuardianGrantScopeSession,
				Request: GuardianRequest{
					ID:         "req-1",
					ToolCallID: "tool-1",
					ToolName:   "bash",
					Action:     GuardianActionExec,
					Command:    "go test ./...",
					WorkingDir: "/work",
					Metadata: map[string]any{
						"risk": "medium",
					},
				},
				Resolution: GuardianResolution{
					Action: GuardianResolutionAllow,
					Scope:  GuardianGrantScopeSession,
					Reason: "approved",
				},
			},
		},
		Pending: []GuardianApproval{
			{
				ID:            "approval-1",
				DecisionID:    "decision-1",
				AllowedScopes: []GuardianGrantScope{GuardianGrantScopeOnce, GuardianGrantScopeSession},
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"current_profile": "team",
		"profiles": {
			"team": {
				"name": "team",
				"description": "team defaults",
				"rules": [
					{
						"actions": ["write", "network"],
						"decision": "ask",
						"reason": "requires approval"
					}
				]
			}
		},
		"overlays": [
			{
				"id": "overlay-1",
				"source": "plan-mode",
				"description": "temporary plan policy",
				"rules": [
					{
						"actions": ["write"],
						"decision": "allow",
						"reason": "trusted plan edits"
					}
				],
				"override_hard_blocks": true
			}
		],
		"grants": [
			{
				"id": "grant-1",
				"scope": "session",
				"request": {
					"id": "req-1",
					"tool_call_id": "tool-1",
					"tool_name": "bash",
					"action": "exec",
					"command": "go test ./...",
					"working_dir": "/work",
					"metadata": {
						"risk": "medium"
					}
				},
				"resolution": {
					"action": "allow",
					"scope": "session",
					"reason": "approved"
				}
			}
		],
		"pending": [
			{
				"id": "approval-1",
				"decision_id": "decision-1",
				"request": {
					"id": "",
					"tool_name": "",
					"action": ""
				},
				"allowed_scopes": ["once", "session"]
			}
		]
	}`, string(data))

	var got GuardianSnapshot
	require.NoError(t, json.Unmarshal([]byte(`{
		"current_profile": "team",
		"profiles": {
			"team": {
				"name": "team",
				"description": "team defaults",
				"rules": [
					{
						"actions": ["write", "network"],
						"decision": "ask",
						"reason": "requires approval"
					}
				]
			}
		},
		"overlays": [
			{
				"id": "overlay-1",
				"source": "plan-mode",
				"description": "temporary plan policy",
				"rules": [
					{
						"actions": ["write"],
						"decision": "allow",
						"reason": "trusted plan edits"
					}
				],
				"override_hard_blocks": true
			}
		],
		"grants": [
			{
				"id": "grant-1",
				"scope": "session",
				"request": {
					"id": "req-1",
					"tool_call_id": "tool-1",
					"tool_name": "bash",
					"action": "exec",
					"command": "go test ./...",
					"working_dir": "/work",
					"metadata": {
						"risk": "medium"
					}
				},
				"resolution": {
					"action": "allow",
					"scope": "session",
					"reason": "approved"
				}
			}
		],
		"pending": [
			{
				"id": "approval-1",
				"decision_id": "decision-1",
				"request": {
					"id": "",
					"tool_name": "",
					"action": ""
				},
				"allowed_scopes": ["once", "session"]
			}
		]
	}`), &got))

	assert.Equal(t, payload.CurrentProfile, got.CurrentProfile)
	require.Contains(t, got.Profiles, "team")
	assert.Equal(t, GuardianDecisionAsk, got.Profiles["team"].Rules[0].Decision)
	require.Len(t, got.Overlays, 1)
	assert.Equal(t, "overlay-1", got.Overlays[0].ID)
	assert.Equal(t, GuardianDecisionAllow, got.Overlays[0].Rules[0].Decision)
	assert.True(t, got.Overlays[0].OverrideHardBlocks)
	require.Len(t, got.Grants, 1)
	assert.Equal(t, GuardianActionExec, got.Grants[0].Request.Action)
	assert.Equal(t, GuardianGrantScopeSession, got.Grants[0].Resolution.Scope)
	require.Len(t, got.Pending, 1)
	assert.Equal(t, []GuardianGrantScope{GuardianGrantScopeOnce, GuardianGrantScopeSession}, got.Pending[0].AllowedScopes)
}

func TestGuardianSnapshotZeroValueJSONCompatibility(t *testing.T) {
	data, err := json.Marshal(GuardianSnapshot{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"current_profile":""}`, string(data))

	var got GuardianSnapshot
	require.NoError(t, json.Unmarshal([]byte(`{"current_profile":"auto"}`), &got))
	assert.Equal(t, "auto", got.CurrentProfile)
	assert.Nil(t, got.Profiles)
	assert.Nil(t, got.Overlays)
	assert.Nil(t, got.Grants)
	assert.Nil(t, got.Pending)
}

func TestGuardianPolicyOverlayJSONRoundTrip(t *testing.T) {
	payload := GuardianPolicyOverlay{
		ID:          "overlay-1",
		Source:      "plan-mode",
		Description: "allow plan edits",
		Rules: []GuardianProfileRule{
			{
				Actions:  []GuardianAction{GuardianActionWrite},
				Decision: GuardianDecisionAllow,
				Reason:   "trusted plan overlay",
				Metadata: map[string]any{
					"scope": "session",
				},
			},
			{
				Actions:  []GuardianAction{GuardianActionDelete},
				Decision: GuardianDecisionBlock,
			},
		},
		OverrideHardBlocks: true,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "overlay-1",
		"source": "plan-mode",
		"description": "allow plan edits",
		"rules": [
			{
				"actions": ["write"],
				"decision": "allow",
				"reason": "trusted plan overlay",
				"metadata": {
					"scope": "session"
				}
			},
			{
				"actions": ["delete"],
				"decision": "block"
			}
		],
		"override_hard_blocks": true
	}`, string(data))

	var got GuardianPolicyOverlay
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "overlay-1",
		"source": "plan-mode",
		"description": "allow plan edits",
		"rules": [
			{
				"actions": ["write"],
				"decision": "allow",
				"reason": "trusted plan overlay",
				"metadata": {
					"scope": "session"
				}
			},
			{
				"actions": ["delete"],
				"decision": "block"
			}
		],
		"override_hard_blocks": true
	}`), &got))

	assert.Equal(t, payload.ID, got.ID)
	assert.Equal(t, payload.Source, got.Source)
	assert.Equal(t, payload.Description, got.Description)
	assert.True(t, got.OverrideHardBlocks)
	require.Len(t, got.Rules, 2)
	assert.Equal(t, GuardianActionWrite, got.Rules[0].Actions[0])
	assert.Equal(t, GuardianDecisionAllow, got.Rules[0].Decision)
	assert.Equal(t, "session", got.Rules[0].Metadata["scope"])
	assert.Equal(t, GuardianDecisionBlock, got.Rules[1].Decision)
}

func TestGuardianPolicyOverlayPopJSONRoundTrip(t *testing.T) {
	payload := GuardianPolicyOverlayPop{
		ID:     "overlay-1",
		Source: "plan-mode",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "overlay-1",
		"source": "plan-mode"
	}`, string(data))

	var got GuardianPolicyOverlayPop
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "overlay-1",
		"source": "plan-mode"
	}`), &got))

	assert.Equal(t, payload.ID, got.ID)
	assert.Equal(t, payload.Source, got.Source)
}
