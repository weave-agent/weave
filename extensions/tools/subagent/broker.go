package subagent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Protocol message type constants.
const (
	msgTypeSend           = "send"
	msgTypeBroadcast      = "broadcast"
	msgTypeListAgents     = "list_agents"
	msgTypeListAgentsResp = "list_agents_response"
	msgTypeAgentMsg       = "agent_msg"
	msgTypeInject         = "inject"
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

// subagentProc tracks an active subagent process managed by the broker.
type subagentProc struct {
	id    string
	name  string
	stdin io.Writer
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
func (b *Broker) Register(id, name string, stdin io.Writer) {
	b.mu.Lock()
	b.agents[id] = &subagentProc{
		id:    id,
		name:  name,
		stdin: stdin,
	}
	b.mu.Unlock()

	b.injectRoster(id)
}

// Unregister removes a subagent from the broker's registry.
func (b *Broker) Unregister(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.agents, id)
}

// MonitorStdout reads JSON lines from the agent's stdout, routes inter-agent
// messages, and captures the final result from the last "message_end" event.
// When the reader closes (process exits), the agent is automatically unregistered.
func (b *Broker) MonitorStdout(id string, stdout io.Reader) (string, error) {
	result, err := b.monitorStdout(id, stdout)
	b.Unregister(id)

	return result, err
}

func (b *Broker) monitorStdout(id string, stdout io.Reader) (string, error) {
	scanner := bufio.NewScanner(stdout)

	var finalContent string

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
			_ = b.routeSend(id, msg.To, msg.Content)
		case msgTypeBroadcast:
			b.routeBroadcast(id, msg.Content)
		case msgTypeListAgents:
			b.respondListAgents(id)
		case msgTypeMessageEnd:
			finalContent = msg.Content
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stdout: %w", err)
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

	return b.writeMessage(target.stdin, msg)
}

// routeBroadcast forwards a message from one agent to all other active agents.
func (b *Broker) routeBroadcast(fromID, content string) {
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

	for _, proc := range others {
		_ = b.writeMessage(proc.stdin, msg)
	}
}

// respondListAgents writes the current roster back to the requesting agent.
func (b *Broker) respondListAgents(requesterID string) {
	roster := b.Roster()

	msg := brokerMessage{
		Type:   msgTypeListAgentsResp,
		Agents: roster,
	}

	b.mu.RLock()
	proc, ok := b.agents[requesterID]
	b.mu.RUnlock()

	if ok {
		_ = b.writeMessage(proc.stdin, msg)
	}
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

	return b.writeMessage(proc.stdin, msg)
}

// Roster returns a snapshot of all currently registered agents.
func (b *Broker) Roster() []agentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

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

// injectRoster sends the current agent roster to the specified agent's stdin
// as an initial context message (excluding the agent itself).
func (b *Broker) injectRoster(id string) {
	roster := b.Roster()

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

	if ok {
		_ = b.writeMessage(proc.stdin, msg)
	}
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
