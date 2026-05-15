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
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "test", "background")
	tracker.Done("agent-1", "completed", "done")
	tracker.Done("agent-1", "completed", "done again")

	time.Sleep(150 * time.Millisecond)

	// onRemove should only fire once — second Done is a no-op.
	assert.Equal(t, int32(1), removed.Load())
}

func TestAgentTracker_GracePeriod(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Done("agent-1", "completed", "done")

	// Agent still present during grace period
	assert.NotNil(t, tracker.Get("agent-1"))

	// Wait for grace period to fire
	time.Sleep(150 * time.Millisecond)

	// Agent should have been removed
	assert.Nil(t, tracker.Get("agent-1"))
	assert.Equal(t, int32(1), removed.Load())
}

func TestAgentTracker_GracePeriod_RemoveCancels(t *testing.T) {
	var removed atomic.Int32

	tracker := NewAgentTracker(50*time.Millisecond, func(id string) {
		removed.Add(1)
	})

	tracker.Start("agent-1", "researcher", "background")
	tracker.Done("agent-1", "completed", "done")

	// Explicitly remove before grace period fires
	tracker.Remove("agent-1")

	time.Sleep(150 * time.Millisecond)

	// onRemove should NOT have been called since we removed explicitly
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

	// Grace-period timer should have been canceled.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), removed.Load())
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
