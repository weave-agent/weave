package sdk

import (
	"context"
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
	assert.Equal(t, "guardian.snapshot.request", GuardianSnapshotRequestTopic)
	assert.Equal(t, "guardian.snapshot", GuardianSnapshotTopic)
	assert.Equal(t, "guardian.grants.clear", GuardianClearGrantsTopic)
}
