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

// AgentTracker manages the lifecycle of tracked subagents.
type AgentTracker struct {
	mu          sync.RWMutex
	agents      map[string]*TrackedAgent
	timers      map[string]*time.Timer
	onRemove    func(id string)
	gracePeriod time.Duration
}

// NewAgentTracker creates a new tracker. The onRemove callback is invoked
// when the grace period expires after an agent finishes. May be nil.
func NewAgentTracker(gracePeriod time.Duration, onRemove func(id string)) *AgentTracker {
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

// SetOnRemove sets the callback invoked after grace-period cleanup.
func (t *AgentTracker) SetOnRemove(fn func(id string)) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.onRemove = fn
}

// Start registers a new running agent. Returns the created TrackedAgent.
// If an agent with the same ID already exists, it is overwritten (the old
// agent and any active grace-period timer are cleaned up first).
func (t *AgentTracker) Start(id, name, mode string) *TrackedAgent {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clean up any existing agent with the same ID to prevent leaks.
	if old, ok := t.agents[id]; ok {
		if timer, hasTimer := t.timers[id]; hasTimer {
			timer.Stop()
			delete(t.timers, id)
		}
		delete(t.agents, id)
		_ = old // explicitly acknowledge we're dropping the old agent
	}

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

	// Guard against double-Done calls — agent already in terminal state.
	if agent.Status != AgentRunning {
		return
	}

	switch status {
	case statusCompleted:
		agent.Status = AgentCompleted
	case statusFailed:
		agent.Status = AgentFailed
	default:
		agent.Status = AgentFailed
	}

	agent.Result = result
	agent.DoneAt = time.Now()

	onRemove := t.onRemove

	timer := time.AfterFunc(t.gracePeriod, func() {
		t.mu.Lock()
		delete(t.agents, id)
		delete(t.timers, id)
		t.mu.Unlock()

		if onRemove != nil {
			onRemove(id)
		}
	})
	t.timers[id] = timer
}

// Get returns a snapshot copy of a tracked agent by ID, or nil if not found.
// Returns a value (not pointer) so callers can read fields without races.
func (t *AgentTracker) Get(id string) *TrackedAgent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	a, ok := t.agents[id]
	if !ok {
		return nil
	}

	cp := *a

	return &cp
}

// List returns snapshot copies of all tracked agents.
func (t *AgentTracker) List() []*TrackedAgent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*TrackedAgent, 0, len(t.agents))
	for _, a := range t.agents {
		cp := *a
		result = append(result, &cp)
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

// Close stops all grace-period timers and removes all tracked agents.
// It is safe to call multiple times.
func (t *AgentTracker) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, timer := range t.timers {
		timer.Stop()
		delete(t.timers, id)
	}

	for id := range t.agents {
		delete(t.agents, id)
	}
}
