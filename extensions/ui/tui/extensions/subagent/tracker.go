package subagent

import (
	"sync"
	"time"
)

// AgentStatus represents the current state of a tracked subagent.
type AgentStatus int

const (
	AgentRunning AgentStatus = iota
	AgentCompleted
	AgentFailed
)

// TrackedAgent holds the state of a single subagent being tracked.
type TrackedAgent struct {
	ID        string
	Name      string
	Status    AgentStatus
	Mode      string
	SpawnedAt time.Time
	DoneAt    time.Time
	Result    string
	PanelID   string
}

// OnRemoveFunc is called when a grace-period timer expires and the agent
// should be cleaned up. The extension uses this to call RemovePanel.
type OnRemoveFunc func(id string)

// AgentTracker manages the lifecycle of tracked subagents.
type AgentTracker struct {
	mu     sync.RWMutex
	agents map[string]*TrackedAgent
	timers map[string]*time.Timer
	onRemove OnRemoveFunc
	gracePeriod time.Duration
}

// NewAgentTracker creates a new tracker. The onRemove callback is invoked
// when the grace period expires after an agent finishes.
func NewAgentTracker(gracePeriod time.Duration, onRemove OnRemoveFunc) *AgentTracker {
	if gracePeriod <= 0 {
		gracePeriod = 3 * time.Second
	}
	return &AgentTracker{
		agents:      make(map[string]*TrackedAgent),
		timers:      make(map[string]*time.Timer),
		onRemove:    onRemove,
		gracePeriod: gracePeriod,
	}
}

// Start registers a new running agent. Returns the created TrackedAgent.
func (t *AgentTracker) Start(id, name, mode string) *TrackedAgent {
	t.mu.Lock()
	defer t.mu.Unlock()

	agent := &TrackedAgent{
		ID:        id,
		Name:      name,
		Status:    AgentRunning,
		Mode:      mode,
		SpawnedAt: time.Now(),
		PanelID:   "subagent-" + id,
	}
	t.agents[id] = agent
	return agent
}

// Done marks an agent as completed or failed and starts the grace-period
// timer. After the grace period, onRemove is called and the agent is removed.
func (t *AgentTracker) Done(id, status, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	agent, ok := t.agents[id]
	if !ok {
		return
	}

	switch status {
	case "completed":
		agent.Status = AgentCompleted
	case "failed":
		agent.Status = AgentFailed
	default:
		agent.Status = AgentFailed
	}
	agent.Result = result
	agent.DoneAt = time.Now()

	if t.onRemove != nil {
		timer := time.AfterFunc(t.gracePeriod, func() {
			t.mu.Lock()
			delete(t.agents, id)
			delete(t.timers, id)
			t.mu.Unlock()
			t.onRemove(id)
		})
		t.timers[id] = timer
	}
}

// Get returns a tracked agent by ID, or nil if not found.
func (t *AgentTracker) Get(id string) *TrackedAgent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.agents[id]
}

// List returns all tracked agents.
func (t *AgentTracker) List() []*TrackedAgent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*TrackedAgent, 0, len(t.agents))
	for _, a := range t.agents {
		result = append(result, a)
	}
	return result
}

// Remove immediately removes a tracked agent and cancels its grace-period timer.
func (t *AgentTracker) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if timer, ok := t.timers[id]; ok {
		timer.Stop()
		delete(t.timers, id)
	}
	delete(t.agents, id)
}
