package tui

import (
	"testing"

	"weave/bus"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateEvent_TurnStart(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicTurnStart, 3))
	ts, ok := msg.(TurnStartMsg)
	require.True(t, ok)
	assert.Equal(t, 3, ts.Turn)
}

func TestTranslateEvent_TurnEnd(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicTurnEnd, nil))
	_, ok := msg.(TurnEndMsg)
	require.True(t, ok)
}

func TestTranslateEvent_MessageStart(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicMsgStart, nil))
	_, ok := msg.(MessageStartMsg)
	require.True(t, ok)
}

func TestTranslateEvent_MessageUpdate(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicMsgUpdate, "hello "))
	mu, ok := msg.(MessageUpdateMsg)
	require.True(t, ok)
	assert.Equal(t, "hello ", mu.Content)
}

func TestTranslateEvent_MessageEnd(t *testing.T) {
	payload := map[string]any{
		"content":    "response text",
		"tool_calls": []sdk.ToolCall{{ID: "tc1", Name: "bash"}},
	}

	msg := translateEvent(sdk.NewEvent(topicMsgEnd, payload))
	me, ok := msg.(MessageEndMsg)
	require.True(t, ok)
	assert.Equal(t, "response text", me.Content)
	require.Len(t, me.ToolCalls, 1)
	assert.Equal(t, "tc1", me.ToolCalls[0].ID)
	assert.Equal(t, "bash", me.ToolCalls[0].Name)
}

func TestTranslateEvent_MessageEnd_NilPayload(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicMsgEnd, nil))
	me, ok := msg.(MessageEndMsg)
	require.True(t, ok)
	assert.Equal(t, "", me.Content)
	assert.Nil(t, me.ToolCalls)
}

func TestTranslateEvent_MessageEnd_WithThinking(t *testing.T) {
	payload := map[string]any{
		"content":  "response text",
		"thinking": "I considered the alternatives...",
		"tool_calls": []sdk.ToolCall{},
	}

	msg := translateEvent(sdk.NewEvent(topicMsgEnd, payload))
	me, ok := msg.(MessageEndMsg)
	require.True(t, ok)
	assert.Equal(t, "response text", me.Content)
	assert.Equal(t, "I considered the alternatives...", me.Thinking)
}

func TestTranslateEvent_MessageEnd_WithoutThinking(t *testing.T) {
	payload := map[string]any{
		"content":    "response text",
		"tool_calls": []sdk.ToolCall{},
	}

	msg := translateEvent(sdk.NewEvent(topicMsgEnd, payload))
	me, ok := msg.(MessageEndMsg)
	require.True(t, ok)
	assert.Equal(t, "", me.Thinking)
}

func TestTranslateEvent_ToolResult(t *testing.T) {
	payload := map[string]any{
		"id":     "tc1",
		"tool":   "bash",
		"result": sdk.ToolResult{Content: "output", IsError: false},
	}

	msg := translateEvent(sdk.NewEvent(topicToolResult, payload))
	tr, ok := msg.(ToolResultMsg)
	require.True(t, ok)
	assert.Equal(t, "tc1", tr.ToolID)
	assert.Equal(t, "bash", tr.Tool)
	assert.Equal(t, "output", tr.Result.Content)
	assert.False(t, tr.Result.IsError)
}

func TestTranslateEvent_ToolResult_NilPayload(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicToolResult, nil))
	tr, ok := msg.(ToolResultMsg)
	require.True(t, ok)
	assert.Equal(t, "", tr.ToolID)
	assert.Equal(t, "", tr.Tool)
}

func TestTranslateEvent_AgentEnd(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicEnd, "stream error: timeout"))
	ae, ok := msg.(AgentEndMsg)
	require.True(t, ok)
	assert.Equal(t, "stream error: timeout", ae.Payload)
}

func TestTranslateEvent_AgentEnd_NilPayload(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicEnd, nil))
	ae, ok := msg.(AgentEndMsg)
	require.True(t, ok)
	assert.Nil(t, ae.Payload)
}

func TestTranslateEvent_UnknownTopic(t *testing.T) {
	msg := translateEvent(sdk.NewEvent("unknown.topic", "data"))
	assert.Nil(t, msg)
}

func TestTranslateEvent_MessageUpdate_NonStringPayload(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicMsgUpdate, 42))
	mu, ok := msg.(MessageUpdateMsg)
	require.True(t, ok)
	assert.Equal(t, "", mu.Content)
}

func TestTranslateEvent_TurnStart_NonIntPayload(t *testing.T) {
	msg := translateEvent(sdk.NewEvent(topicTurnStart, "not an int"))
	ts, ok := msg.(TurnStartMsg)
	require.True(t, ok)
	assert.Equal(t, 0, ts.Turn)
}

func TestBridge_ForwardsEventsAndShutdown(t *testing.T) {
	sender := &collectingSender{}

	events := make(chan sdk.Event, 5)

	done := make(chan struct{})

	go func() {
		Bridge(sender, events)
		close(done)
	}()

	events <- sdk.NewEvent(topicMsgStart, nil)
	events <- sdk.NewEvent(topicMsgUpdate, "hello")
	events <- sdk.NewEvent(topicTurnEnd, nil)

	close(events)

	<-done

	require.Len(t, sender.msgs, 4) // 3 events + shutdown

	_, ok := sender.msgs[0].(MessageStartMsg)
	assert.True(t, ok)

	mu, ok := sender.msgs[1].(MessageUpdateMsg)
	assert.True(t, ok)
	assert.Equal(t, "hello", mu.Content)

	_, ok = sender.msgs[2].(TurnEndMsg)
	assert.True(t, ok)

	_, ok = sender.msgs[3].(ShutdownMsg)
	assert.True(t, ok)
}

func TestBridge_SkipsUnknownTopics(t *testing.T) {
	sender := &collectingSender{}

	events := make(chan sdk.Event, 5)

	done := make(chan struct{})

	go func() {
		Bridge(sender, events)
		close(done)
	}()

	events <- sdk.NewEvent("unknown.topic", "data")
	events <- sdk.NewEvent(topicMsgStart, nil)

	close(events)

	<-done

	require.Len(t, sender.msgs, 2) // unknown skipped, msg start + shutdown

	_, ok := sender.msgs[0].(MessageStartMsg)
	assert.True(t, ok)

	_, ok = sender.msgs[1].(ShutdownMsg)
	assert.True(t, ok)
}

func TestBridge_IntegrationWithRealBus(t *testing.T) {
	b := bus.New()
	defer b.Close()

	events := b.SubscribeAll()

	sender := &collectingSender{}

	done := make(chan struct{})

	go func() {
		Bridge(sender, events)
		close(done)
	}()

	b.Publish(sdk.NewEvent(topicTurnStart, 1))
	b.Publish(sdk.NewEvent(topicMsgStart, nil))
	b.Publish(sdk.NewEvent(topicMsgUpdate, "hi"))
	b.Publish(sdk.NewEvent(topicMsgEnd, map[string]any{"content": "hi", "tool_calls": []sdk.ToolCall{}}))
	b.Publish(sdk.NewEvent(topicTurnEnd, nil))
	b.Publish(sdk.NewEvent(topicEnd, nil))

	b.Unsubscribe(events)

	<-done

	require.Len(t, sender.msgs, 7) // 6 events + shutdown

	assert.IsType(t, TurnStartMsg{}, sender.msgs[0])
	assert.IsType(t, MessageStartMsg{}, sender.msgs[1])

	mu, ok := sender.msgs[2].(MessageUpdateMsg)
	require.True(t, ok)
	assert.Equal(t, "hi", mu.Content)

	assert.IsType(t, MessageEndMsg{}, sender.msgs[3])
	assert.IsType(t, TurnEndMsg{}, sender.msgs[4])
	assert.IsType(t, AgentEndMsg{}, sender.msgs[5])
	assert.IsType(t, ShutdownMsg{}, sender.msgs[6])
}

func TestPublishPrompt(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicPrompt)

	cmd := PublishPrompt(b, "hello world")
	result := cmd()
	assert.Nil(t, result)

	evt := <-ch
	assert.Equal(t, topicPrompt, evt.Topic)
	assert.Equal(t, "hello world", evt.Payload)
}

func TestPublishFollowup(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicFollowup)

	cmd := PublishFollowup(b, "follow up text")
	result := cmd()
	assert.Nil(t, result)

	evt := <-ch
	assert.Equal(t, topicFollowup, evt.Topic)
	assert.Equal(t, "follow up text", evt.Payload)
}

func TestPublishSteer(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicSteer)

	cmd := PublishSteer(b, "steer text")
	result := cmd()
	assert.Nil(t, result)

	evt := <-ch
	assert.Equal(t, topicSteer, evt.Topic)
	assert.Equal(t, "steer text", evt.Payload)
}

// collectingSender captures Send calls for testing.
type collectingSender struct {
	msgs []tea.Msg
}

func (c *collectingSender) Send(msg tea.Msg) {
	c.msgs = append(c.msgs, msg)
}
