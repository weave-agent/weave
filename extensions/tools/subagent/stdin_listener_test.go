package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBus records published events for test verification.
type mockBus struct {
	mu     sync.Mutex
	events []sdk.Event
}

func newMockBus() *mockBus {
	return &mockBus{events: make([]sdk.Event, 0)}
}

func (b *mockBus) Publish(e sdk.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, e)
}

func (b *mockBus) On(string, sdk.Handler) {}
func (b *mockBus) OnAll(sdk.Handler)      {}
func (b *mockBus) Off(sdk.Handler)        {}
func (b *mockBus) Close() error           { return nil }

func (b *mockBus) published(topic string) []sdk.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []sdk.Event

	for _, e := range b.events {
		if e.Topic == topic {
			result = append(result, e)
		}
	}

	return result
}

func setupStdinListener(t *testing.T) (*mockBus, *io.PipeWriter, func()) {
	t.Helper()

	t.Setenv("WEAVE_SUBAGENT_ID", "subagent_test_123")

	pr, pw := io.Pipe()
	oldStdin := stdinReader
	stdinReader = pr

	bus := newMockBus()
	startStdinListener(bus)

	cleanup := func() {
		_ = pw.Close()

		stopStdinListener()

		stdinReader = oldStdin
	}

	return bus, pw, cleanup
}

func TestStdinListener_InjectMessage(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, `{"type":"inject","content":"focus on tests"}`)
	_ = pw.Close()

	// Wait for the listener to process the line and exit.
	stopStdinListener()

	events := bus.published(topicSteer)
	require.Len(t, events, 1)
	assert.Equal(t, "focus on tests", events[0].Payload)
}

func TestStdinListener_AgentMsg(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, `{"type":"agent_msg","from":"agent1","content":"found it"}`)
	_ = pw.Close()

	stopStdinListener()

	events := bus.published(topicSteer)
	require.Len(t, events, 1)
	assert.Equal(t, "[from agent1] found it", events[0].Payload)
}

func TestStdinListener_Cancel(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, `{"type":"cancel"}`)
	_ = pw.Close()

	stopStdinListener()

	events := bus.published(topicInterrupt)
	require.Len(t, events, 1)
}

func TestStdinListener_ListAgentsResponse(t *testing.T) {
	_, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	sl := getStdinListener()
	require.NotNil(t, sl)

	respCh := make(chan string, 1)
	sl.setResponseChannel(respCh)

	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "a1", Name: "explore", Status: "running"},
		},
	}
	data, _ := json.Marshal(response)
	_, _ = fmt.Fprintln(pw, string(data))
	_ = pw.Close()

	stopStdinListener()

	select {
	case result := <-respCh:
		assert.Contains(t, result, "a1")
		assert.Contains(t, result, "explore")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for list_agents_response")
	}
}

func TestStdinListener_SkipsNonJSON(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, "not json")
	_, _ = fmt.Fprintln(pw, `{"type":"inject","content":"valid"}`)
	_ = pw.Close()

	stopStdinListener()

	events := bus.published(topicSteer)
	require.Len(t, events, 1)
	assert.Equal(t, "valid", events[0].Payload)
}

func TestStdinListener_SkipsUnknownTypes(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, `{"type":"unknown_type","content":"ignored"}`)
	_ = pw.Close()

	stopStdinListener()

	assert.Empty(t, bus.published(topicSteer))
	assert.Empty(t, bus.published(topicInterrupt))
}

func TestStdinListener_DoesNotStartWithoutEnvVar(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "")

	bus := newMockBus()
	startStdinListener(bus)

	assert.Nil(t, getStdinListener())
}

func TestStdinListener_GracefulShutdown(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	// Send a message, then close the pipe.
	_, _ = fmt.Fprintln(pw, `{"type":"inject","content":"test"}`)
	_ = pw.Close()

	// Stop should complete without hanging.
	done := make(chan struct{})

	go func() {
		stopStdinListener()
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(2 * time.Second):
		t.Fatal("stopStdinListener timed out")
	}

	events := bus.published(topicSteer)
	require.Len(t, events, 1)
}

func TestStdinListener_MultipleMessages(t *testing.T) {
	bus, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	_, _ = fmt.Fprintln(pw, `{"type":"inject","content":"first"}`)
	_, _ = fmt.Fprintln(pw, `{"type":"agent_msg","from":"a1","content":"second"}`)
	_, _ = fmt.Fprintln(pw, `{"type":"cancel"}`)
	_ = pw.Close()

	stopStdinListener()

	steerEvents := bus.published(topicSteer)
	require.Len(t, steerEvents, 2)
	assert.Equal(t, "first", steerEvents[0].Payload)
	assert.Equal(t, "[from a1] second", steerEvents[1].Payload)

	interruptEvents := bus.published(topicInterrupt)
	require.Len(t, interruptEvents, 1)
}

func TestReadListAgentsResponse_WithListener(t *testing.T) {
	_, pw, cleanup := setupStdinListener(t)
	defer cleanup()

	// Prepare response.
	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "agent1", Name: "explore", Status: "running"},
		},
	}
	data, _ := json.Marshal(response)

	// Write response in background.
	go func() {
		time.Sleep(50 * time.Millisecond)

		_, _ = fmt.Fprintln(pw, string(data))
		_ = pw.Close()
	}()

	result, err := readListAgentsResponse(context.Background(), nil)
	require.NoError(t, err)
	assert.Contains(t, result, "agent1")
	assert.Contains(t, result, "explore")
}

func TestReadListAgentsResponse_WithoutListener_Fallback(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "")

	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "a1", Name: "explore", Status: "running"},
		},
	}
	data, _ := json.Marshal(response)

	result, err := readListAgentsResponse(context.Background(), strings.NewReader(string(data)+"\n"))
	require.NoError(t, err)
	assert.Contains(t, result, "a1")
}

func TestReadListAgentsResponse_WithoutListener_ContextCanceled(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readListAgentsResponse(ctx, strings.NewReader(""))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
