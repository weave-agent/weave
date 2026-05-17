package subagent

import (
	"sync"
	"time"
)

// outputEntry represents a single subagent output event stored in the ring buffer.
type outputEntry struct {
	Type    string // "tool_call", "tool_result", "message_start", "message_update", "message_end"
	Tool    string // e.g. "read", "edit"
	Content string // truncated content
	Time    time.Time
}

// outputRing is a thread-safe ring buffer that retains the last N output entries.
type outputRing struct {
	mu    sync.RWMutex
	items []outputEntry
	cap   int
	head  int // next write position
	full  bool
}

// newOutputRing creates a ring buffer with the given capacity.
func newOutputRing(capacity int) *outputRing {
	if capacity <= 0 {
		capacity = 200
	}

	return &outputRing{
		items: make([]outputEntry, capacity),
		cap:   capacity,
	}
}

// Append adds an entry to the ring buffer.
func (r *outputRing) Append(entry outputEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[r.head] = entry
	r.head = (r.head + 1) % r.cap

	if r.head == 0 {
		r.full = true
	}
}

// Snapshot returns all entries in insertion order (oldest first).
func (r *outputRing) Snapshot() []outputEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := r.len()
	if n == 0 {
		return nil
	}

	result := make([]outputEntry, n)

	if r.full {
		copy(result, r.items[r.head:])
		copy(result[r.cap-r.head:], r.items[:r.head])
	} else {
		copy(result, r.items[:r.head])
	}

	return result
}

// Len returns the number of entries in the ring buffer.
func (r *outputRing) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.len()
}

func (r *outputRing) len() int {
	if r.full {
		return r.cap
	}

	return r.head
}

// PanelIDForAgent returns the panel ID for a given agent ID.
func PanelIDForAgent(id string) string {
	return "subagent-" + id
}

// AgentStatus represents the current state of a tracked subagent.
type AgentStatus int

const (
	AgentRunning AgentStatus = iota
	AgentCompleted
	AgentFailed
	AgentCancelled
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
	Output    *outputRing
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

	// Clean up any existing agent with the same ID to prevent leaks.
	var (
		oldID  string
		hadOld bool
	)

	if old, ok := t.agents[id]; ok {
		if timer, hasTimer := t.timers[id]; hasTimer {
			timer.Stop()
			delete(t.timers, id)
		}

		delete(t.agents, id)

		oldID = old.ID
		hadOld = true
	}

	agent := &TrackedAgent{
		ID:        id,
		Name:      name,
		Status:    AgentRunning,
		Mode:      mode,
		SpawnedAt: time.Now(),
		PanelID:   PanelIDForAgent(id),
		Output:    newOutputRing(200),
	}
	t.agents[id] = agent
	onRemove := t.onRemove
	t.mu.Unlock()

	if hadOld && onRemove != nil {
		onRemove(oldID)
	}

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
	case statusCancelled:
		agent.Status = AgentCancelled
	default:
		agent.Status = AgentFailed
	}

	agent.Result = result
	agent.DoneAt = time.Now()

	onRemove := t.onRemove

	timer := time.AfterFunc(t.gracePeriod, func() {
		t.mu.Lock()
		_, hadAgent := t.agents[id]
		delete(t.agents, id)
		delete(t.timers, id)
		t.mu.Unlock()

		if onRemove != nil && hadAgent {
			onRemove(id)
		}
	})
	t.timers[id] = timer
}

// AppendOutput adds an output entry to the tracked agent's ring buffer.
// Returns false if the agent is not found.
func (t *AgentTracker) AppendOutput(id string, entry outputEntry) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	a, ok := t.agents[id]
	if ok {
		a.Output.Append(entry)
	}

	return ok
}

// Get returns a snapshot copy of a tracked agent by ID, or nil if not found.
// Scalar fields are safe to read without races. The Output field is a shared
// pointer protected by its own mutex — call Output.Snapshot() for a safe copy.
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

	onRemove := t.onRemove
	for id := range t.agents {
		delete(t.agents, id)

		if onRemove != nil {
			onRemove(id)
		}
	}
}
