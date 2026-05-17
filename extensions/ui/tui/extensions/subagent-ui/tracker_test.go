package subagent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTracker_Start(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	agent := tracker.Start("agent-1", "researcher", "background")

	assert.Equal(t, "agent-1", agent.ID)
	assert.Equal(t, "researcher", agent.Name)
	assert.Equal(t, "background", agent.Mode)
	assert.Equal(t, AgentRunning, agent.Status)
	assert.Equal(t, "subagent-agent-1", agent.PanelID)
	assert.WithinDuration(t, time.Now(), agent.SpawnedAt, time.Second)
}

func TestAgentTracker_Get(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	tracker.Start("agent-1", "researcher", "background")

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)
	assert.Equal(t, "researcher", agent.Name)

	assert.Nil(t, tracker.Get("nonexistent"))
}

func TestAgentTracker_List(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	tracker.Start("agent-1", "researcher", "background")
	tracker.Start("agent-2", "planner", "background")

	list := tracker.List()
	assert.Len(t, list, 2)

	names := map[string]bool{}
	for _, a := range list {
		names[a.Name] = true
	}

	assert.True(t, names["researcher"])
	assert.True(t, names["planner"])
}

func TestAgentTracker_ListEmpty(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	assert.Empty(t, tracker.List())
}

func TestAgentTracker_Start_OverwriteExisting(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "original", "background")
	tracker.Done("agent-1", "completed", "old result")

	// Start a new agent with the same ID — old agent and timer should be cleaned up.
	tracker.Start("agent-1", "replacement", "background")

	assert.Equal(t, "replacement", tracker.Get("agent-1").Name)

	// Old agent was removed immediately by overwrite — callback should have fired once.
	assert.Equal(t, int32(1), removed.Load())
}

func TestAgentTracker_Start_OverwriteRunning(t *testing.T) {
	removedCh := make(chan string, 2)

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removedCh <- id
	})

	// Start a running agent (no grace timer yet).
	tracker.Start("agent-1", "original", "background")

	// Overwrite with a new agent before calling Done.
	// This triggers onRemove for the old agent immediately.
	tracker.Start("agent-1", "replacement", "background")

	assert.Equal(t, "replacement", tracker.Get("agent-1").Name)
	assert.Equal(t, AgentRunning, tracker.Get("agent-1").Status)

	// Completing the replacement should trigger the callback once more after grace period.
	tracker.Done("agent-1", "completed", "done")

	// First callback: immediate removal of overwritten agent.
	select {
	case id := <-removedCh:
		assert.Equal(t, "agent-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for overwrite callback")
	}

	// Second callback: grace-period removal after Done.
	select {
	case id := <-removedCh:
		assert.Equal(t, "agent-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for grace-period callback")
	}

	assert.Nil(t, tracker.Get("agent-1"))
}

func TestAgentTracker_Done_NilOnRemove(t *testing.T) {
	tracker := NewAgentTracker(50*time.Millisecond, nil)

	tracker.Start("agent-1", "test", "background")
	tracker.Done("agent-1", "completed", "done")

	// Should not panic with nil onRemove. Poll until agent is removed or timeout.
	require.Eventually(t, func() bool {
		return tracker.Get("agent-1") == nil
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAgentTracker_Remove(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	tracker.Start("agent-1", "researcher", "background")
	require.NotNil(t, tracker.Get("agent-1"))

	tracker.Remove("agent-1")
	assert.Nil(t, tracker.Get("agent-1"))
}

func TestAgentTracker_RemoveNonexistent(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	assert.NotPanics(t, func() {
		tracker.Remove("nonexistent")
	})
}

func TestAgentTracker_Done_Completed(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "researcher", "background")

	tracker.Done("agent-1", "completed", "result text")

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)
	assert.Equal(t, AgentCompleted, agent.Status)
	assert.Equal(t, "result text", agent.Result)
	assert.WithinDuration(t, time.Now(), agent.DoneAt, time.Second)
}

func TestAgentTracker_Done_Failed(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "researcher", "background")

	tracker.Done("agent-1", "failed", "error message")

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)
	assert.Equal(t, AgentFailed, agent.Status)
	assert.Equal(t, "error message", agent.Result)
}

func TestAgentTracker_Done_UnknownStatus(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "researcher", "background")

	tracker.Done("agent-1", "something_else", "data")

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)
	assert.Equal(t, AgentFailed, agent.Status)
}

func TestAgentTracker_Done_Nonexistent(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	assert.NotPanics(t, func() {
		tracker.Done("nonexistent", "completed", "")
	})
}

func TestAgentTracker_Done_CalledTwice(t *testing.T) {
	removedCh := make(chan string, 1)

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removedCh <- id
	})

	tracker.Start("agent-1", "test", "background")
	tracker.Done("agent-1", "completed", "done")
	tracker.Done("agent-1", "completed", "done again")

	select {
	case <-removedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for grace-period callback")
	}

	// onRemove should only fire once — second Done is a no-op.
	select {
	case <-removedCh:
		t.Fatal("unexpected second callback")
	default:
	}

	assert.Nil(t, tracker.Get("agent-1"))
}

func TestAgentTracker_GracePeriod(t *testing.T) {
	removedCh := make(chan string, 1)

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removedCh <- id
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Done("agent-1", "completed", "done")

	// Agent still present during grace period
	assert.NotNil(t, tracker.Get("agent-1"))

	// Wait for grace period to fire
	select {
	case <-removedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for grace-period callback")
	}

	// Agent should have been removed
	assert.Nil(t, tracker.Get("agent-1"))
}

func TestAgentTracker_GracePeriod_RemoveCancels(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Done("agent-1", "completed", "done")

	// Explicitly remove before grace period fires — timer is stopped.
	tracker.Remove("agent-1")

	// onRemove should NOT have been called since we removed explicitly.
	assert.Equal(t, int32(0), removed.Load())
}

func TestAgentTracker_DefaultGracePeriod(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	assert.Equal(t, 3*time.Second, tracker.gracePeriod)
}

func TestAgentTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	var wg sync.WaitGroup

	for n := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			id := fmt.Sprintf("agent-%d", n)
			tracker.Start(id, "test", "background")
			tracker.Get(id)
			tracker.List()
			tracker.Remove(id)
		}(n)
	}

	wg.Wait()
	assert.Empty(t, tracker.List())
}

func TestAgentTracker_Close(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Start("agent-2", "planner", "background")
	tracker.Done("agent-1", "completed", "done")

	assert.Len(t, tracker.List(), 2)

	tracker.Close()

	assert.Empty(t, tracker.List())
	assert.Nil(t, tracker.Get("agent-1"))
	assert.Nil(t, tracker.Get("agent-2"))

	// Both agents removed immediately by Close — callback should fire twice.
	assert.Equal(t, int32(2), removed.Load())
}

func TestAgentTracker_Close_DuringGracePeriod(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(2*time.Second, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Done("agent-1", "completed", "done")

	// Agent is still in tracker during grace period
	assert.NotNil(t, tracker.Get("agent-1"))

	// Close before grace period expires — should call onRemove exactly once
	tracker.Close()

	assert.Nil(t, tracker.Get("agent-1"))
	assert.Equal(t, int32(1), removed.Load())
}

func TestAgentTracker_Close_Idempotent(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "test", "background")

	assert.NotPanics(t, func() {
		tracker.Close()
		tracker.Close()
		tracker.Close()
	})
}

func TestOutputRing_AppendAndSnapshot(t *testing.T) {
	ring := newOutputRing(5)

	assert.Nil(t, ring.Snapshot())
	assert.Equal(t, 0, ring.Len())

	now := time.Now()
	for i := range 3 {
		ring.Append(outputEntry{Type: "tool_call", Tool: "read", Content: fmt.Sprintf("entry-%d", i), Time: now})
	}

	assert.Equal(t, 3, ring.Len())

	snap := ring.Snapshot()
	require.Len(t, snap, 3)
	assert.Equal(t, "entry-0", snap[0].Content)
	assert.Equal(t, "entry-1", snap[1].Content)
	assert.Equal(t, "entry-2", snap[2].Content)
}

func TestOutputRing_WrapAround(t *testing.T) {
	ring := newOutputRing(3)

	for i := range 6 {
		ring.Append(outputEntry{Content: fmt.Sprintf("entry-%d", i)})
	}

	assert.Equal(t, 3, ring.Len())

	snap := ring.Snapshot()
	require.Len(t, snap, 3)

	// Should contain last 3 entries (3, 4, 5)
	assert.Equal(t, "entry-3", snap[0].Content)
	assert.Equal(t, "entry-4", snap[1].Content)
	assert.Equal(t, "entry-5", snap[2].Content)
}

func TestOutputRing_WrapAroundExactMultiple(t *testing.T) {
	ring := newOutputRing(3)

	for i := range 3 {
		ring.Append(outputEntry{Content: fmt.Sprintf("entry-%d", i)})
	}

	assert.Equal(t, 3, ring.Len())

	snap := ring.Snapshot()
	require.Len(t, snap, 3)
	assert.Equal(t, "entry-0", snap[0].Content)
	assert.Equal(t, "entry-1", snap[1].Content)
	assert.Equal(t, "entry-2", snap[2].Content)
}

func TestOutputRing_DefaultCapacity(t *testing.T) {
	ring := newOutputRing(0)
	assert.Equal(t, 200, ring.cap)
}

func TestOutputRing_NegativeCapacity(t *testing.T) {
	ring := newOutputRing(-5)
	assert.Equal(t, 200, ring.cap)
}

func TestOutputRing_SnapshotEmpty(t *testing.T) {
	ring := newOutputRing(10)
	assert.Nil(t, ring.Snapshot())
	assert.Equal(t, 0, ring.Len())
}

func TestOutputRing_ConcurrentAppendRead(t *testing.T) {
	ring := newOutputRing(100)

	var wg sync.WaitGroup

	// Concurrent writers
	for w := range 10 {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()

			for i := range 50 {
				ring.Append(outputEntry{Content: fmt.Sprintf("w%d-i%d", w, i)})
			}
		}(w)
	}

	// Concurrent readers
	for range 5 {
		wg.Add(1)

		//nolint:modernize // standard WaitGroup pattern
		go func() {
			defer wg.Done()

			for range 100 {
				snap := ring.Snapshot()

				// Snapshot should always be ordered and at most 100 entries
				assert.LessOrEqual(t, len(snap), 100)
			}
		}()
	}

	wg.Wait()

	// Final state: exactly 100 entries (capacity)
	assert.Equal(t, 100, ring.Len())

	snap := ring.Snapshot()
	assert.Len(t, snap, 100)
}

func TestOutputRing_FieldsPreserved(t *testing.T) {
	ring := newOutputRing(10)
	ts := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	ring.Append(outputEntry{
		Type:    "tool_call",
		Tool:    "read",
		Content: "main.go",
		Time:    ts,
	})

	snap := ring.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "tool_call", snap[0].Type)
	assert.Equal(t, "read", snap[0].Tool)
	assert.Equal(t, "main.go", snap[0].Content)
	assert.Equal(t, ts, snap[0].Time)
}

func TestTrackedAgent_OutputInitialized(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	agent := tracker.Start("agent-1", "researcher", "background")

	require.NotNil(t, agent.Output)
	assert.Equal(t, 0, agent.Output.Len())

	// Verify Output is usable
	agent.Output.Append(outputEntry{Type: "message_start", Content: "hello"})
	assert.Equal(t, 1, agent.Output.Len())
}

func TestTrackedAgent_OutputSharedViaGet(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	agent := tracker.Start("agent-1", "researcher", "background")

	agent.Output.Append(outputEntry{Content: "before-get"})

	// Get returns a shallow copy — Output pointer should be the same ring
	got := tracker.Get("agent-1")
	require.NotNil(t, got)
	require.NotNil(t, got.Output)

	// Append after Get — both should see it
	got.Output.Append(outputEntry{Content: "after-get"})

	snap := agent.Output.Snapshot()
	require.Len(t, snap, 2)
	assert.Equal(t, "before-get", snap[0].Content)
	assert.Equal(t, "after-get", snap[1].Content)
}

func TestAgentTracker_AppendOutput(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "researcher", "background")

	ok := tracker.AppendOutput("agent-1", outputEntry{
		Type:    "tool_call",
		Tool:    "read",
		Content: "main.go",
	})
	assert.True(t, ok)

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)

	snap := agent.Output.Snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "tool_call", snap[0].Type)
	assert.Equal(t, "read", snap[0].Tool)
	assert.Equal(t, "main.go", snap[0].Content)
}

func TestAgentTracker_AppendOutput_MissingAgent(t *testing.T) {
	tracker := NewAgentTracker(0, nil)

	ok := tracker.AppendOutput("nonexistent", outputEntry{
		Type:    "tool_call",
		Content: "test",
	})
	assert.False(t, ok)
}

func TestAgentTracker_AppendOutput_MultipleEntries(t *testing.T) {
	tracker := NewAgentTracker(0, nil)
	tracker.Start("agent-1", "researcher", "background")

	entries := []outputEntry{
		{Type: "tool_call", Tool: "read", Content: "a.go"},
		{Type: "tool_result", Tool: "read", Content: "result"},
		{Type: "message_update", Content: "thinking..."},
	}

	for _, e := range entries {
		tracker.AppendOutput("agent-1", e)
	}

	agent := tracker.Get("agent-1")
	require.NotNil(t, agent)

	snap := agent.Output.Snapshot()
	require.Len(t, snap, 3)
	assert.Equal(t, "tool_call", snap[0].Type)
	assert.Equal(t, "tool_result", snap[1].Type)
	assert.Equal(t, "message_update", snap[2].Type)
}
