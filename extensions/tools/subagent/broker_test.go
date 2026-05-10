package subagent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startDrainer starts a goroutine that reads JSON lines from r and sends them
// on the returned channel.
func startDrainer(r io.Reader) <-chan brokerMessage {
	ch := make(chan brokerMessage, 10)

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			var msg brokerMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
				ch <- msg
			}
		}

		close(ch)
	}()

	return ch
}

// discardReader starts a goroutine that discards all reads from r.
func discardReader(r io.Reader) {
	go func() {
		_, _ = io.Copy(io.Discard, r)
	}()
}

func TestBroker_RouteSend(t *testing.T) {
	broker := NewBroker()

	// Register agent2 first so it exists when agent1 sends to it.
	a2StdinR, a2StdinW := io.Pipe()
	a2Drain := startDrainer(a2StdinR)

	broker.Register("agent2", "coder", a2StdinW)
	<-a2Drain // consume roster injection (empty for first agent)

	// Register agent1.
	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // consume roster injection (contains agent2)

	// Simulate agent1 sending a message to agent2.
	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"send","to":"agent2","content":"hello from agent1"}`)
		_ = a1StdoutW.Close()
	}()

	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Empty(t, result)

	// Verify agent2 received the routed message.
	msg := <-a2Drain
	assert.Equal(t, "agent_msg", msg.Type)
	assert.Equal(t, "agent1", msg.From)
	assert.Equal(t, "hello from agent1", msg.Content)
}

func TestBroker_RouteSend_TargetNotFound(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // consume roster injection

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"send","to":"nonexistent","content":"hello"}`)
		_ = a1StdoutW.Close()
	}()

	// Should not panic; returns empty result since no message_end.
	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBroker_Broadcast(t *testing.T) {
	broker := NewBroker()

	// Register three agents.
	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // empty roster

	a2StdinR, a2StdinW := io.Pipe()
	a2Drain := startDrainer(a2StdinR)

	broker.Register("agent2", "coder", a2StdinW)
	<-a2Drain // roster with agent1

	a3StdinR, a3StdinW := io.Pipe()
	a3Drain := startDrainer(a3StdinR)

	broker.Register("agent3", "plan", a3StdinW)
	<-a3Drain // roster with agent1, agent2

	// Agent1 broadcasts.
	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"broadcast","content":"hello all"}`)
		_ = a1StdoutW.Close()
	}()

	_, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)

	// Agent2 and agent3 should receive the broadcast.
	msg2 := <-a2Drain
	assert.Equal(t, "agent_msg", msg2.Type)
	assert.Equal(t, "agent1", msg2.From)
	assert.Equal(t, "hello all", msg2.Content)

	msg3 := <-a3Drain
	assert.Equal(t, "agent_msg", msg3.Type)
	assert.Equal(t, "agent1", msg3.From)
	assert.Equal(t, "hello all", msg3.Content)

	// Agent1 should NOT receive its own broadcast.
	select {
	case <-a1Drain:
		t.Fatal("agent1 should not receive its own broadcast")
	case <-time.After(50 * time.Millisecond):
		// Expected.
	}
}

func TestBroker_Broadcast_NoOthers(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // empty roster

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"broadcast","content":"hello all"}`)
		_ = a1StdoutW.Close()
	}()

	// Should not panic when there are no other agents.
	_, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
}

func TestBroker_ListAgents(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // roster injection

	a2StdinR, a2StdinW := io.Pipe()
	a2Drain := startDrainer(a2StdinR)

	broker.Register("agent2", "coder", a2StdinW)
	<-a2Drain // roster injection

	// Agent1 requests list_agents.
	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"list_agents"}`)
		_ = a1StdoutW.Close()
	}()

	_, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)

	// Agent1 should receive list_agents_response.
	msg := <-a1Drain
	assert.Equal(t, "list_agents_response", msg.Type)
	require.Len(t, msg.Agents, 2)

	ids := make([]string, len(msg.Agents))
	for i, a := range msg.Agents {
		ids[i] = a.ID
	}

	assert.Contains(t, ids, "agent1")
	assert.Contains(t, ids, "agent2")
}

func TestBroker_Inject(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // consume roster injection

	err := broker.Inject("agent1", "injected context")
	require.NoError(t, err)

	msg := <-a1Drain
	assert.Equal(t, "inject", msg.Type)
	assert.Equal(t, "injected context", msg.Content)
}

func TestBroker_Inject_NotFound(t *testing.T) {
	broker := NewBroker()
	err := broker.Inject("nonexistent", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBroker_RosterInjection(t *testing.T) {
	broker := NewBroker()

	// Agent1 registered first: empty roster.
	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)

	msg := <-a1Drain
	assert.Equal(t, "agent_msg", msg.Type)
	assert.Contains(t, msg.Content, "No other agents")

	// Agent2 registered second: roster contains agent1.
	a2StdinR, a2StdinW := io.Pipe()
	a2Drain := startDrainer(a2StdinR)

	broker.Register("agent2", "coder", a2StdinW)

	msg = <-a2Drain
	assert.Equal(t, "agent_msg", msg.Type)
	assert.Contains(t, msg.Content, "agent1")
	assert.Contains(t, msg.Content, "explore")

	// Verify agents field is populated.
	require.Len(t, msg.Agents, 1)
	assert.Equal(t, "agent1", msg.Agents[0].ID)
	assert.Equal(t, "explore", msg.Agents[0].Name)
}

func TestBroker_MonitorStdout_CapturesResult(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_start","model":"gpt-5"}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_update","content":"working..."}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_end","content":"final answer"}`)
		_ = a1StdoutW.Close()
	}()

	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Equal(t, "final answer", result)
}

func TestBroker_MonitorStdout_MultipleMessageEnd_LastWins(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_end","content":"first"}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_end","content":"second"}`)
		_ = a1StdoutW.Close()
	}()

	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Equal(t, "second", result)
}

func TestBroker_MonitorStdout_NonJSONIgnored(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, "log: starting")
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_end","content":"result"}`)
		_, _ = fmt.Fprintln(a1StdoutW, "log: done")
		_ = a1StdoutW.Close()
	}()

	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Equal(t, "result", result)
}

func TestBroker_MonitorStdout_UnregistersOnDone(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	// Verify agent is registered.
	roster := broker.Roster()
	require.Len(t, roster, 1)

	a1StdoutR, a1StdoutW := io.Pipe()
	_ = a1StdoutW.Close() // Immediately close.

	_, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)

	// Verify agent is unregistered.
	roster = broker.Roster()
	assert.Empty(t, roster)
}

func TestBroker_Roster(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	a2StdinR, a2StdinW := io.Pipe()
	discardReader(a2StdinR)
	broker.Register("agent2", "coder", a2StdinW)

	roster := broker.Roster()
	require.Len(t, roster, 2)

	names := make(map[string]string)
	for _, a := range roster {
		names[a.ID] = a.Name
	}

	assert.Equal(t, "explore", names["agent1"])
	assert.Equal(t, "coder", names["agent2"])
}

func TestBroker_MonitorStdout_WithRoutingEvents(t *testing.T) {
	broker := NewBroker()

	// Setup two agents.
	a2StdinR, a2StdinW := io.Pipe()
	a2Drain := startDrainer(a2StdinR)

	broker.Register("agent2", "coder", a2StdinW)
	<-a2Drain // roster

	a1StdinR, a1StdinW := io.Pipe()
	a1Drain := startDrainer(a1StdinR)

	broker.Register("agent1", "explore", a1StdinW)
	<-a1Drain // roster

	a1StdoutR, a1StdoutW := io.Pipe()
	go func() {
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_start"}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"send","to":"agent2","content":"found it"}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_update","content":"almost done"}`)
		_, _ = fmt.Fprintln(a1StdoutW, `{"type":"message_end","content":"final result"}`)
		_ = a1StdoutW.Close()
	}()

	result, err := broker.MonitorStdout("agent1", a1StdoutR)
	require.NoError(t, err)
	assert.Equal(t, "final result", result)

	// Verify agent2 got the routed message.
	msg := <-a2Drain
	assert.Equal(t, "agent_msg", msg.Type)
	assert.Equal(t, "found it", msg.Content)
}

func TestBroker_Unregister(t *testing.T) {
	broker := NewBroker()

	a1StdinR, a1StdinW := io.Pipe()
	discardReader(a1StdinR)
	broker.Register("agent1", "explore", a1StdinW)

	require.Len(t, broker.Roster(), 1)

	broker.Unregister("agent1")
	require.Empty(t, broker.Roster())

	// Unregistering non-existent agent should not panic.
	broker.Unregister("nonexistent")
}

func TestFormatRoster_Empty(t *testing.T) {
	result := formatRoster(nil)
	assert.Equal(t, "No other agents are currently running.", result)
}

func TestFormatRoster_Populated(t *testing.T) {
	agents := []agentInfo{
		{ID: "agent1", Name: "explore"},
		{ID: "agent2", Name: "coder"},
	}
	result := formatRoster(agents)
	assert.Contains(t, result, "agent1 (explore)")
	assert.Contains(t, result, "agent2 (coder)")
}
