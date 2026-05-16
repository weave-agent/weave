package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBus is a minimal mock of sdk.Bus that captures published events.
type testBus struct {
	mu     sync.Mutex
	events []sdk.Event
}

func (b *testBus) Publish(e sdk.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
}

func (b *testBus) On(string, sdk.Handler) {}
func (b *testBus) OnAll(sdk.Handler)      {}
func (b *testBus) Off(sdk.Handler)        {}
func (b *testBus) Close() error           { return nil }
func (b *testBus) getEvents() []sdk.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]sdk.Event, len(b.events))
	copy(out, b.events)

	return out
}

func TestBackgroundSpawn_ReturnsImmediately(t *testing.T) {
	original := testRunSubagent

	// Slow mock to prove we return before completion.
	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "done: " + prompt, nil
	}

	mgr := newBackgroundManager(nil, "", "")

	t.Cleanup(func() {
		mgr.cancel()

		testRunSubagent = original
	})

	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil, "", "")

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "background": true}

	start := time.Now()
	result, err := tool.Execute(ctx, args)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Less(t, elapsed, 100*time.Millisecond, "expected immediate return")

	// Verify JSON response.
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &resp))
	assert.Equal(t, "running", resp["status"])
	assert.NotEmpty(t, resp["id"])
	id := resp["id"].(string)
	assert.Contains(t, id, "subagent_test_")

	// Wait for background to complete.
	time.Sleep(300 * time.Millisecond)

	ba, ok := mgr.get(id)
	require.True(t, ok)
	assert.Equal(t, "completed", ba.Status)
	assert.Equal(t, "done: hello", ba.Result)
}

func TestBackgroundSpawn_CompletesWithError(t *testing.T) {
	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "", errors.New("mock failure")
	}

	mgr := newBackgroundManager(nil, "", "")
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil, "", "")

	t.Cleanup(func() {
		mgr.cancel()

		testRunSubagent = original
	})

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "background": true}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &resp))
	id := resp["id"].(string)

	// Wait for completion.
	time.Sleep(50 * time.Millisecond)

	ba, ok := mgr.get(id)
	require.True(t, ok)
	assert.Equal(t, "failed", ba.Status)
	assert.Contains(t, ba.Result, "mock failure")
}

func TestCheckAgent_Pending(t *testing.T) {
	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	mgr := newBackgroundManager(nil, "", "")
	id, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		mgr.cancel()

		ba, ok := mgr.get(id)
		if ok {
			<-ba.done
		}

		testRunSubagent = original
	})

	tool := &checkAgentTool{mgr: mgr}
	result, err := tool.Execute(context.Background(), map[string]any{"id": id})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &resp))
	assert.Equal(t, id, resp["id"])
	assert.Equal(t, "running", resp["status"])
	assert.Nil(t, resp["content"], "pending agent should not have content")
}

func TestCheckAgent_Completed(t *testing.T) {
	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "final result", nil
	}

	mgr := newBackgroundManager(nil, "", "")
	id, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		mgr.cancel()

		ba, ok := mgr.get(id)
		if ok {
			<-ba.done
		}

		testRunSubagent = original
	})

	// Wait for completion.
	time.Sleep(50 * time.Millisecond)

	tool := &checkAgentTool{mgr: mgr}
	result, err := tool.Execute(context.Background(), map[string]any{"id": id})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &resp))
	assert.Equal(t, id, resp["id"])
	assert.Equal(t, "completed", resp["status"])
	assert.Equal(t, "final result", resp["content"])
}

func TestCheckAgent_NotFound(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	tool := &checkAgentTool{mgr: mgr}

	result, err := tool.Execute(context.Background(), map[string]any{"id": "nonexistent"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not found")
}

func TestCheckAgent_MissingID(t *testing.T) {
	tool := &checkAgentTool{mgr: newBackgroundManager(nil, "", "")}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter")
}

func TestCheckAgent_InvalidIDType(t *testing.T) {
	tool := &checkAgentTool{mgr: newBackgroundManager(nil, "", "")}

	result, err := tool.Execute(context.Background(), map[string]any{"id": 123})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "non-empty string")
}

func TestAwaitAgent_BlocksUntilDone(t *testing.T) {
	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "final result", nil
	}

	mgr := newBackgroundManager(nil, "", "")
	id, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		mgr.cancel()

		ba, ok := mgr.get(id)
		if ok {
			<-ba.done
		}

		testRunSubagent = original
	})

	tool := &awaitAgentTool{mgr: mgr}

	start := time.Now()
	result, err := tool.Execute(context.Background(), map[string]any{"id": id})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond, "expected to block until completion")

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &resp))
	assert.Equal(t, "completed", resp["status"])
	assert.Equal(t, "final result", resp["content"])
}

func TestAwaitAgent_ContextCancellation(t *testing.T) {
	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	mgr := newBackgroundManager(nil, "", "")
	id, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		mgr.cancel()

		ba, ok := mgr.get(id)
		if ok {
			<-ba.done
		}

		testRunSubagent = original
	})

	tool := &awaitAgentTool{mgr: mgr}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := tool.Execute(ctx, map[string]any{"id": id})
	require.NoError(t, err)
	// When context is canceled, await returns a cancellation error.
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "canceled")
}

func TestAwaitAgent_NotFound(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	tool := &awaitAgentTool{mgr: mgr}

	result, err := tool.Execute(context.Background(), map[string]any{"id": "nonexistent"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not found")
}

func TestAwaitAgent_MissingID(t *testing.T) {
	tool := &awaitAgentTool{mgr: newBackgroundManager(nil, "", "")}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter")
}

func TestBackgroundManager_NotifyDone(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil, "", "")
	mgr.setBus(bus)

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "result", nil
	}

	t.Cleanup(func() {
		mgr.cancel()

		testRunSubagent = original
	})

	_, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	// Wait for completion.
	time.Sleep(50 * time.Millisecond)

	events := bus.getEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "subagent.started", events[0].Topic)
	assert.Equal(t, "subagent.done", events[1].Topic)

	startedPayload, ok := events[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Contains(t, startedPayload["id"], "subagent_test_")
	assert.Equal(t, "test", startedPayload["name"])
	assert.Equal(t, "background", startedPayload["mode"])

	donePayload, ok := events[1].Payload.(map[string]string)
	require.True(t, ok)
	assert.Contains(t, donePayload["id"], "subagent_test_")
	assert.Equal(t, "completed", donePayload["status"])
	assert.Equal(t, "result", donePayload["content"])
}

func TestBackgroundManager_NotifyDoneWithError(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil, "", "")
	mgr.setBus(bus)

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "", errors.New("failed")
	}

	t.Cleanup(func() {
		mgr.cancel()

		testRunSubagent = original
	})

	_, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	events := bus.getEvents()
	require.Len(t, events, 2)

	donePayload, ok := events[1].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "failed", donePayload["status"])
	assert.Contains(t, donePayload["content"], "failed")
}

func TestBackgroundManager_NotifyDoneNoBus(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	// No bus set.

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "result", nil
	}

	t.Cleanup(func() {
		mgr.cancel()

		testRunSubagent = original
	})

	// Should not panic.
	id, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	ba, ok := mgr.get(id)
	require.True(t, ok)
	assert.Equal(t, "completed", ba.Status)
}

func TestSubagentTool_BackgroundNotAvailable(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "") // nil manager

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "background": true}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "background manager not available")
}

func TestSubagentTool_BackgroundParallelError(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil, "", "")

	ctx := context.Background()
	args := map[string]any{
		"tasks":      []any{map[string]any{"prompt": "task"}},
		"background": true,
	}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not supported for parallel")
}

func TestSubagentTool_BackgroundChainError(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil, "", "")

	ctx := context.Background()
	args := map[string]any{
		"chain":      []any{map[string]any{"prompt": "step"}},
		"background": true,
	}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not supported for chain")
}

func TestGenerateAgentID(t *testing.T) {
	id1 := generateAgentID("explore")
	id2 := generateAgentID("explore")

	assert.Greater(t, len(id1), len("subagent_explore_"))
	assert.Greater(t, len(id2), len("subagent_explore_"))
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "subagent_explore_")
}

func TestCheckAgentTool_Definition(t *testing.T) {
	tool := &checkAgentTool{mgr: newBackgroundManager(nil, "", "")}
	assert.Equal(t, "check_agent", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "check_agent", def.Name)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.Parameters)
}

func TestAwaitAgentTool_Definition(t *testing.T) {
	tool := &awaitAgentTool{mgr: newBackgroundManager(nil, "", "")}
	assert.Equal(t, "await_agent", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "await_agent", def.Name)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.Parameters)
}

func TestBackgroundManager_NotifyStarted(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil, "", "")
	mgr.setBus(bus)

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	var id string

	t.Cleanup(func() {
		mgr.cancel()

		if ba, ok := mgr.get(id); ok && ba != nil {
			<-ba.done
		}

		testRunSubagent = original
	})

	var err error

	id, err = mgr.spawn(&AgentDef{Name: "researcher"}, "find stuff", "", "")
	require.NoError(t, err)

	// The started event should be published synchronously by spawn.
	events := bus.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "subagent.started", events[0].Topic)

	payload, ok := events[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Contains(t, payload["id"], "subagent_researcher_")
	assert.Equal(t, "researcher", payload["name"])
	assert.Equal(t, "background", payload["mode"])
}

func TestBackgroundManager_NotifyStartedNoBus(t *testing.T) {
	mgr := newBackgroundManager(nil, "", "")
	// No bus — spawn should not panic.

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	var id string

	t.Cleanup(func() {
		mgr.cancel()

		if ba, ok := mgr.get(id); ok && ba != nil {
			<-ba.done
		}

		testRunSubagent = original
	})

	var err error

	id, err = mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}

func TestBackgroundManager_NotifyStartedWithCustomID(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil, "", "")
	mgr.setBus(bus)

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	var id string

	t.Cleanup(func() {
		mgr.cancel()

		if ba, ok := mgr.get(id); ok && ba != nil {
			<-ba.done
		}

		testRunSubagent = original
	})

	var err error

	id, err = mgr.spawn(&AgentDef{Name: "explore"}, "prompt", "", "custom_id_123")
	require.NoError(t, err)

	events := bus.getEvents()
	require.Len(t, events, 1)

	payload, ok := events[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "custom_id_123", payload["id"])
	assert.Equal(t, "explore", payload["name"])
	assert.Equal(t, "background", payload["mode"])
}

func TestBackgroundManager_Spawn_IDCollision(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil, "", "")
	mgr.setBus(bus)

	original := testRunSubagent

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	t.Cleanup(func() {
		mgr.cancel()

		for _, ba := range mgr.agents {
			if ba != nil {
				<-ba.done
			}
		}

		testRunSubagent = original
	})

	// Spawn two agents with the same explicit ID — second should get a different ID.
	id1, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt1", "", "same-id")
	require.NoError(t, err)
	id2, err := mgr.spawn(&AgentDef{Name: "test"}, "prompt2", "", "same-id")
	require.NoError(t, err)

	assert.Equal(t, "same-id", id1)
	assert.NotEqual(t, id1, id2, "collision should regenerate ID")
	assert.Contains(t, id2, "subagent_test_")

	// Both agents should be tracked.
	assert.Len(t, bus.getEvents(), 2)
}
