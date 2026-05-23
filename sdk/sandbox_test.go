package sdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxerMock_WrapCommandStatusExpansion(t *testing.T) {
	ctx := context.Background()
	cmdReq := SandboxCommandRequest{
		ID:         "cmd-1",
		ToolCallID: "tool-1",
		Command:    "go test ./sdk/...",
		WorkingDir: "/work",
	}
	expansionReq := SandboxExpansionRequest{
		ID:         "expansion-req-1",
		ToolCallID: "tool-1",
		Command:    cmdReq.Command,
		WorkingDir: cmdReq.WorkingDir,
		Reason:     "needs module cache write access",
		Filesystem: []SandboxFilesystemExpansion{
			{Path: "/Users/andrey/go/pkg/mod", Access: SandboxFilesystemReadWrite},
		},
	}
	resolution := SandboxExpansionResolution{
		State:  SandboxExpansionAllowed,
		Reason: "approved for this command",
	}

	mock := &SandboxerMock{
		WrapCommandFunc: func(context.Context, SandboxCommandRequest) (SandboxCommand, error) {
			return SandboxCommand{
				Command:    "sandbox-exec",
				Args:       []string{"bash", "-lc", cmdReq.Command},
				WorkingDir: cmdReq.WorkingDir,
				Profile:    "seatbelt",
			}, nil
		},
		StatusFunc: func(context.Context) (SandboxStatus, error) {
			return SandboxStatus{
				Availability: SandboxAvailabilityAvailable,
				Provider:     "seatbelt",
				Profile:      "default",
			}, nil
		},
		RequestExpansionFunc: func(context.Context, SandboxExpansionRequest) (SandboxExpansion, error) {
			return SandboxExpansion{
				ID:        "expansion-1",
				RequestID: "expansion-req-1",
				State:     SandboxExpansionPending,
			}, nil
		},
		ResolveExpansionFunc: func(context.Context, string, SandboxExpansionResolution) error {
			return nil
		},
	}

	wrapped, err := mock.WrapCommand(ctx, cmdReq)
	require.NoError(t, err)
	assert.Equal(t, "sandbox-exec", wrapped.Command)
	assert.Equal(t, []string{"bash", "-lc", "go test ./sdk/..."}, wrapped.Args)
	assert.Equal(t, "seatbelt", wrapped.Profile)

	status, err := mock.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, SandboxAvailabilityAvailable, status.Availability)
	assert.Equal(t, "seatbelt", status.Provider)

	expansion, err := mock.RequestExpansion(ctx, expansionReq)
	require.NoError(t, err)
	assert.Equal(t, "expansion-1", expansion.ID)
	assert.Equal(t, SandboxExpansionPending, expansion.State)

	require.NoError(t, mock.ResolveExpansion(ctx, expansion.ID, resolution))

	require.Len(t, mock.WrapCommandCalls(), 1)
	assert.Equal(t, cmdReq, mock.WrapCommandCalls()[0].Req)
	require.Len(t, mock.RequestExpansionCalls(), 1)
	assert.Equal(t, expansionReq, mock.RequestExpansionCalls()[0].Req)
	require.Len(t, mock.ResolveExpansionCalls(), 1)
	assert.Equal(t, resolution, mock.ResolveExpansionCalls()[0].Resolution)
}

func TestSandboxEventTopics(t *testing.T) {
	assert.Equal(t, "sandbox.registered", SandboxRegisteredTopic)
	assert.Equal(t, "sandbox.status.request", SandboxStatusRequestTopic)
	assert.Equal(t, "sandbox.status", SandboxStatusTopic)
	assert.Equal(t, "sandbox.expansion.request", SandboxExpansionRequestTopic)
	assert.Equal(t, "sandbox.expansion.resolution", SandboxExpansionResolutionTopic)
}

func TestSandboxPayloadsJSONRoundTrip(t *testing.T) {
	payload := SandboxExpansionRequest{
		ID:         "expansion-req-1",
		ToolCallID: "tool-1",
		Command:    "go test ./...",
		WorkingDir: "/work",
		Reason:     "needs module cache",
		Filesystem: []SandboxFilesystemExpansion{
			{Path: "/go/pkg/mod", Access: SandboxFilesystemReadWrite},
			{Path: "/repo", Access: SandboxFilesystemRead},
		},
		Network: []SandboxNetworkExpansion{
			{Host: "proxy.example.com", Ports: []string{"443"}, Access: SandboxNetworkConnect},
			{Ports: []string{"127.0.0.1:0"}, Access: SandboxNetworkListen},
		},
		Metadata: map[string]any{
			"provider": "seatbelt",
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var got SandboxExpansionRequest
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, payload.ID, got.ID)
	assert.Equal(t, SandboxFilesystemReadWrite, got.Filesystem[0].Access)
	assert.Equal(t, "/repo", got.Filesystem[1].Path)
	assert.Equal(t, SandboxNetworkConnect, got.Network[0].Access)
	assert.Equal(t, []string{"127.0.0.1:0"}, got.Network[1].Ports)

	resolution := SandboxExpansion{
		ID:        "expansion-1",
		RequestID: payload.ID,
		State:     SandboxExpansionAllowed,
		Resolution: &SandboxExpansionResolution{
			State:     SandboxExpansionAllowed,
			Reason:    "approved once",
			ExpiresAt: "2026-05-23T10:00:00Z",
		},
	}
	data, err = json.Marshal(resolution)
	require.NoError(t, err)

	var expansion SandboxExpansion
	require.NoError(t, json.Unmarshal(data, &expansion))

	assert.Equal(t, SandboxExpansionAllowed, expansion.State)
	require.NotNil(t, expansion.Resolution)
	assert.Equal(t, "approved once", expansion.Resolution.Reason)
}
