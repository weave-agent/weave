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

	t.Cleanup(func() { testRunSubagent = original })

	// Slow mock to prove we return before completion.
	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "done: " + prompt, nil
	}

	mgr := newBackgroundManager(nil)
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil)

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "background": true}

	start := time.Now()
	result, err := tool.Execute(ctx, args)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Less(t, elapsed, 50*time.Millisecond, "expected immediate return")

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

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		return "", errors.New("mock failure")
	}

	mgr := newBackgroundManager(nil)
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil)

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

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		time.Sleep(5 * time.Second)
		return "never", nil
	}

	mgr := newBackgroundManager(nil)
	id := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

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

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		return "final result", nil
	}

	mgr := newBackgroundManager(nil)
	id := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

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
	mgr := newBackgroundManager(nil)
	tool := &checkAgentTool{mgr: mgr}

	result, err := tool.Execute(context.Background(), map[string]any{"id": "nonexistent"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not found")
}

func TestCheckAgent_MissingID(t *testing.T) {
	tool := &checkAgentTool{mgr: newBackgroundManager(nil)}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter")
}

func TestCheckAgent_InvalidIDType(t *testing.T) {
	tool := &checkAgentTool{mgr: newBackgroundManager(nil)}

	result, err := tool.Execute(context.Background(), map[string]any{"id": 123})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "non-empty string")
}

func TestAwaitAgent_BlocksUntilDone(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "final result", nil
	}

	mgr := newBackgroundManager(nil)
	id := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

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

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		time.Sleep(5 * time.Second)
		return "never", nil
	}

	mgr := newBackgroundManager(nil)
	id := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

	tool := &awaitAgentTool{mgr: mgr}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := tool.Execute(ctx, map[string]any{"id": id})
	require.NoError(t, err)
	// When context is canceled, await returns nil agent which we treat as not found.
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not found")
}

func TestAwaitAgent_NotFound(t *testing.T) {
	mgr := newBackgroundManager(nil)
	tool := &awaitAgentTool{mgr: mgr}

	result, err := tool.Execute(context.Background(), map[string]any{"id": "nonexistent"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "not found")
}

func TestAwaitAgent_MissingID(t *testing.T) {
	tool := &awaitAgentTool{mgr: newBackgroundManager(nil)}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter")
}

func TestBackgroundManager_NotifyDone(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil)
	mgr.setBus(bus)

	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		return "result", nil
	}

	mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

	// Wait for completion.
	time.Sleep(50 * time.Millisecond)

	events := bus.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "subagent.done", events[0].Topic)

	payload, ok := events[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Contains(t, payload["id"], "subagent_test_")
	assert.Equal(t, "completed", payload["status"])
	assert.Equal(t, "result", payload["content"])
}

func TestBackgroundManager_NotifyDoneWithError(t *testing.T) {
	bus := &testBus{}

	mgr := newBackgroundManager(nil)
	mgr.setBus(bus)

	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		return "", errors.New("failed")
	}

	mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

	time.Sleep(50 * time.Millisecond)

	events := bus.getEvents()
	require.Len(t, events, 1)

	payload, ok := events[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "failed", payload["status"])
	assert.Contains(t, payload["content"], "failed")
}

func TestBackgroundManager_NotifyDoneNoBus(t *testing.T) {
	mgr := newBackgroundManager(nil)
	// No bus set.

	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker) (string, error) {
		return "result", nil
	}

	// Should not panic.
	id := mgr.spawn(&AgentDef{Name: "test"}, "prompt", "", "")

	time.Sleep(50 * time.Millisecond)

	ba, ok := mgr.get(id)
	require.True(t, ok)
	assert.Equal(t, "completed", ba.Status)
}

func TestSubagentTool_BackgroundNotAvailable(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil) // nil manager

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "background": true}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "background manager not available")
}

func TestSubagentTool_BackgroundParallelError(t *testing.T) {
	mgr := newBackgroundManager(nil)
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil)

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
	mgr := newBackgroundManager(nil)
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, mgr, nil)

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
	tool := &checkAgentTool{mgr: newBackgroundManager(nil)}
	assert.Equal(t, "check_agent", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "check_agent", def.Name)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.Parameters)
}

func TestAwaitAgentTool_Definition(t *testing.T) {
	tool := &awaitAgentTool{mgr: newBackgroundManager(nil)}
	assert.Equal(t, "await_agent", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "await_agent", def.Name)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.Parameters)
}
