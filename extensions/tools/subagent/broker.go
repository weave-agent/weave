package subagent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
)

// Protocol message type constants.
const (
	msgTypeSend           = "send"
	msgTypeBroadcast      = "broadcast"
	msgTypeListAgents     = "list_agents"
	msgTypeListAgentsResp = "list_agents_response"
	msgTypeAgentMsg       = "agent_msg"
	msgTypeInject         = "inject"
	msgTypeCancel         = "cancel"
	msgTypeMessageEnd     = "message_end"
	statusRunning         = "running"
	keyTo                 = "to"
	keyContent            = "content"
)

// brokerMessage represents a JSON message on the inter-agent protocol.
type brokerMessage struct {
	Type    string      `json:"type"`
	To      string      `json:"to,omitempty"`
	From    string      `json:"from,omitempty"`
	Content string      `json:"content,omitempty"`
	Agents  []agentInfo `json:"agents,omitempty"`
}

// agentInfo describes a running agent in the roster.
type agentInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

var brokerGenCounter atomic.Uint64

// subagentProc tracks an active subagent process managed by the broker.
type subagentProc struct {
	id    string
	name  string
	stdin io.WriteCloser
	gen   uint64 // monotonic generation to distinguish re-registrations
}

// Broker routes inter-agent messages between running subagent processes.
type Broker struct {
	mu     sync.RWMutex
	agents map[string]*subagentProc
}

// NewBroker creates a new message broker.
func NewBroker() *Broker {
	return &Broker{
		agents: make(map[string]*subagentProc),
	}
}

// Register adds a subagent process to the broker's registry.
// The caller must supply a writer connected to the child's stdin.
// After registration, the current roster is injected into the new agent.
func (b *Broker) Register(id, name string, stdin io.WriteCloser) {
	if stdin == nil {
		log.Printf("broker: refusing to register %s with nil stdin", id)

		return
	}

	b.mu.Lock()
	b.agents[id] = &subagentProc{
		id:    id,
		name:  name,
		stdin: stdin,
		gen:   brokerGenCounter.Add(1),
	}
	roster := b.snapshotRosterLocked()
	b.mu.Unlock()

	// Inject roster asynchronously so a slow or blocked child
	// cannot stall the caller (which may hold no lock but could
	// be on the critical path of spawning subagents).
	go func() {
		if err := b.injectRoster(id, roster); err != nil {
			log.Printf("broker: inject roster to %s failed: %v", id, err)
		}
	}()
}

// Unregister removes a subagent from the broker's registry.
// The gen parameter ensures only the specific registration is removed,
// preventing races where a new agent with the same ID was registered
// concurrently.
func (b *Broker) Unregister(id string, gen uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p, ok := b.agents[id]; ok && p.gen == gen {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}

		delete(b.agents, id)
	}
}

// MonitorStdout reads JSON lines from the agent's stdout, routes inter-agent
// messages, and captures the final result from the last "message_end" event.
// When the reader closes (process exits), the agent is automatically unregistered.
func (b *Broker) MonitorStdout(id string, stdout io.Reader) (string, error) {
	// Capture the generation of the agent we're about to monitor so that
	// we only unregister this specific registration, not a newer one
	// that may have been created concurrently.
	b.mu.RLock()

	var gen uint64
	if p, ok := b.agents[id]; ok {
		gen = p.gen
	}

	b.mu.RUnlock()

	result, err := b.monitorStdout(id, stdout)
	b.Unregister(id, gen)

	return result, err
}

func (b *Broker) monitorStdout(id string, stdout io.Reader) (string, error) {
	scanner := bufio.NewScanner(stdout)

	// Increase buffer capacity to handle large JSON lines (e.g. message_end
	// events with full assistant content). Default 64 KiB is too small.
	const maxCapacity = 10 * 1024 * 1024 // 10 MB

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	var (
		finalContent  string
		sawMessageEnd bool
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg brokerMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Non-JSON lines are ignored (could be log output).
			continue
		}

		switch msg.Type {
		case msgTypeSend:
			if err := b.routeSend(id, msg.To, msg.Content); err != nil {
				log.Printf("broker: route send failed: %v", err)
			}
		case msgTypeBroadcast:
			for _, err := range b.routeBroadcast(id, msg.Content) {
				log.Printf("broker: route broadcast failed: %v", err)
			}
		case msgTypeListAgents:
			if err := b.respondListAgents(id); err != nil {
				log.Printf("broker: respond list agents failed: %v", err)
			}
		case msgTypeMessageEnd:
			finalContent = msg.Content
			sawMessageEnd = true
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stdout: %w", err)
	}

	if !sawMessageEnd {
		return "", errors.New("subagent exited without producing a message_end event")
	}

	return finalContent, nil
}

// routeSend forwards a message from one agent to another's stdin.
func (b *Broker) routeSend(fromID, toID, content string) error {
	b.mu.RLock()
	target, ok := b.agents[toID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("target agent %q not found", toID)
	}

	msg := brokerMessage{
		Type:    msgTypeAgentMsg,
		From:    fromID,
		Content: content,
	}

	if err := b.writeMessage(target.stdin, msg); err != nil {
		b.Unregister(toID, target.gen)
		return fmt.Errorf("route send: %w", err)
	}

	return nil
}

// routeBroadcast forwards a message from one agent to all other active agents.
// Returns a slice of errors for each failed delivery.
func (b *Broker) routeBroadcast(fromID, content string) []error {
	b.mu.RLock()

	var others []*subagentProc

	for id, proc := range b.agents {
		if id != fromID {
			others = append(others, proc)
		}
	}

	b.mu.RUnlock()

	msg := brokerMessage{
		Type:    msgTypeAgentMsg,
		From:    fromID,
		Content: content,
	}

	var errs []error

	for _, proc := range others {
		if err := b.writeMessage(proc.stdin, msg); err != nil {
			b.Unregister(proc.id, proc.gen)
			errs = append(errs, fmt.Errorf("send to %s: %w", proc.id, err))
		}
	}

	return errs
}

// respondListAgents writes the current roster back to the requesting agent.
func (b *Broker) respondListAgents(requesterID string) error {
	roster := b.Roster()

	msg := brokerMessage{
		Type:   msgTypeListAgentsResp,
		Agents: roster,
	}

	b.mu.RLock()
	proc, ok := b.agents[requesterID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("requester agent %q not found", requesterID)
	}

	if err := b.writeMessage(proc.stdin, msg); err != nil {
		b.Unregister(requesterID, proc.gen)
		return fmt.Errorf("respond list agents: %w", err)
	}

	return nil
}

// Inject sends an inject message to a specific subagent's stdin.
func (b *Broker) Inject(id, content string) error {
	b.mu.RLock()
	proc, ok := b.agents[id]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	msg := brokerMessage{
		Type:    msgTypeInject,
		Content: content,
	}

	if err := b.writeMessage(proc.stdin, msg); err != nil {
		b.Unregister(id, proc.gen)
		return fmt.Errorf("inject: %w", err)
	}

	return nil
}

// Roster returns a snapshot of all currently registered agents.
func (b *Broker) Roster() []agentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.snapshotRosterLocked()
}

func (b *Broker) snapshotRosterLocked() []agentInfo {
	roster := make([]agentInfo, 0, len(b.agents))

	for id, proc := range b.agents {
		roster = append(roster, agentInfo{
			ID:     id,
			Name:   proc.name,
			Status: statusRunning,
		})
	}

	return roster
}

// injectRoster sends the provided roster snapshot to the specified agent's stdin
// as an initial context message (excluding the agent itself).
func (b *Broker) injectRoster(id string, roster []agentInfo) error {
	filtered := make([]agentInfo, 0, len(roster))

	for _, a := range roster {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}

	msg := brokerMessage{
		Type:    msgTypeAgentMsg,
		From:    "broker",
		Content: formatRoster(filtered),
		Agents:  filtered,
	}

	b.mu.RLock()
	proc, ok := b.agents[id]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	if err := b.writeMessage(proc.stdin, msg); err != nil {
		b.Unregister(id, proc.gen)
		return fmt.Errorf("inject roster: %w", err)
	}

	return nil
}

func (b *Broker) writeMessage(w io.Writer, msg brokerMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if _, err = fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

func formatRoster(agents []agentInfo) string {
	if len(agents) == 0 {
		return "No other agents are currently running."
	}

	var sb strings.Builder
	sb.WriteString("Currently running agents:\n")

	for _, a := range agents {
		fmt.Fprintf(&sb, "  - %s (%s)\n", a.ID, a.Name)
	}

	return sb.String()
}
