package tui

import (
	"strings"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

// Bus event topics (matching agent-loop topics).
const (
	topicPrompt     = "agent.prompt"
	topicSteer      = "agent.steer"
	topicFollowup   = "agent.followup"
	topicInterrupt  = "agent.interrupt"

	topicTurnStart  = "agent.turn_start"
	topicTurnEnd    = "agent.turn_end"
	topicMsgStart   = "agent.message_start"
	topicMsgUpdate  = "agent.message_update"
	topicMsgEnd     = "agent.message_end"
	topicToolResult = "agent.tool_result"
	topicEnd        = "agent.end"

	topicSessionList   = "session.list"
	topicSessionResume = "session.resume"
	topicModelChange   = "model.change"
)

// Sender abstracts tea.Program.Send for testability.
type Sender interface {
	Send(msg tea.Msg)
}

// tea.Msg types for Bubble Tea.

type TurnStartMsg struct {
	Turn int
}

type TurnEndMsg struct{}

type MessageStartMsg struct{}

type MessageUpdateMsg struct {
	Content string
}

type MessageEndMsg struct {
	Content   string
	Thinking  string
	ToolCalls []sdk.ToolCall
}

type ToolResultMsg struct {
	ToolID string
	Tool   string
	Result sdk.ToolResult
}

type AgentEndMsg struct {
	Payload any
}

type ShutdownMsg struct{}

// SessionListResultMsg carries the result of listing sessions.
type SessionListResultMsg struct {
	Sessions []SessionEntry
	Err      error
}

// SessionResumedMsg is sent when a session resume event arrives from the bus.
type SessionResumedMsg struct {
	SessionID string
}

// ModelListResultMsg carries the result of listing available models.
type ModelListResultMsg struct {
	Models []ModelEntry
}

// ModelChangedMsg is sent when the user selects or cycles to a new model.
type ModelChangedMsg struct {
	Entry ModelEntry
}

// translateEvent converts a bus event into a tea.Msg.
// Returns nil for unknown topics.
func translateEvent(evt sdk.Event) tea.Msg {
	switch evt.Topic {
	case topicTurnStart:
		turn, _ := evt.Payload.(int)
		return TurnStartMsg{Turn: turn}
	case topicTurnEnd:
		return TurnEndMsg{}
	case topicMsgStart:
		return MessageStartMsg{}
	case topicMsgUpdate:
		content, _ := evt.Payload.(string)
		return MessageUpdateMsg{Content: content}
	case topicMsgEnd:
		return translateMsgEnd(evt.Payload)
	case topicToolResult:
		return translateToolResult(evt.Payload)
	case topicEnd:
		return AgentEndMsg{Payload: evt.Payload}
	case topicSessionResume:
		id, _ := evt.Payload.(string)
		return SessionResumedMsg{SessionID: id}
	default:
		return nil
	}
}

func translateMsgEnd(payload any) MessageEndMsg {
	m, ok := payload.(map[string]any)
	if !ok {
		return MessageEndMsg{}
	}

	content, _ := m["content"].(string)
	thinking, _ := m["thinking"].(string)

	var toolCalls []sdk.ToolCall

	if tc, ok := m["tool_calls"].([]sdk.ToolCall); ok {
		toolCalls = tc
	}

	return MessageEndMsg{Content: content, Thinking: thinking, ToolCalls: toolCalls}
}

func translateToolResult(payload any) ToolResultMsg {
	m, ok := payload.(map[string]any)
	if !ok {
		return ToolResultMsg{}
	}

	id, _ := m["id"].(string)
	tool, _ := m["tool"].(string)

	result, ok := m["result"].(sdk.ToolResult)
	if !ok {
		result = sdk.ToolResult{}
	}

	return ToolResultMsg{ToolID: id, Tool: tool, Result: result}
}

// Bridge reads bus events and sends them as tea.Msg to the program.
// When multiple MessageUpdateMsg deltas arrive in rapid succession, it batches
// them into a single concatenated message to reduce UI update pressure.
// Blocks until the event channel is closed.
func Bridge(sender Sender, events <-chan sdk.Event) {
	for evt := range events {
		msg := translateEvent(evt)
		if msg == nil {
			continue
		}

		// Batch consecutive MessageUpdateMsg deltas
		if _, ok := msg.(MessageUpdateMsg); ok {
			var batch strings.Builder
			mu, _ := msg.(MessageUpdateMsg)
			batch.WriteString(mu.Content)

			// Drain any queued deltas
			draining := true
			for draining {
				select {
				case next, ok := <-events:
					if !ok {
						// Channel closed while batching — flush and exit
						if batch.Len() > 0 {
							sender.Send(MessageUpdateMsg{Content: batch.String()})
						}
						sender.Send(ShutdownMsg{})
						return
					}
					nextMsg := translateEvent(next)
					if nextMu, ok := nextMsg.(MessageUpdateMsg); ok {
						batch.WriteString(nextMu.Content)
					} else {
						// Non-delta message — flush the batch, then handle this message
						if batch.Len() > 0 {
							sender.Send(MessageUpdateMsg{Content: batch.String()})
							batch.Reset()
						}
						if nextMsg != nil {
							sender.Send(nextMsg)
						}
						draining = false
					}
				default:
					draining = false
				}
			}

			if batch.Len() > 0 {
				sender.Send(MessageUpdateMsg{Content: batch.String()})
			}
			continue
		}

		sender.Send(msg)
	}

	sender.Send(ShutdownMsg{})
}

// PublishPrompt returns a tea.Cmd that publishes an agent.prompt event.
func PublishPrompt(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicPrompt, text))
		return nil
	}
}

// PublishFollowup returns a tea.Cmd that publishes an agent.followup event.
func PublishFollowup(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicFollowup, text))
		return nil
	}
}

// PublishSteer returns a tea.Cmd that publishes an agent.steer event.
func PublishSteer(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicSteer, text))
		return nil
	}
}

// PublishInterrupt returns a tea.Cmd that publishes an agent.interrupt event.
func PublishInterrupt(bus sdk.Bus) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicInterrupt, "user interrupt"))
		return nil
	}
}

// PublishSessionResume returns a tea.Cmd that publishes a session.resume event.
func PublishSessionResume(bus sdk.Bus, sessionID string) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicSessionResume, sessionID))
		return nil
	}
}

// PublishModelChange returns a tea.Cmd that publishes a model.change event.
func PublishModelChange(bus sdk.Bus, entry ModelEntry) tea.Cmd {
	return func() tea.Msg {
		bus.Publish(sdk.NewEvent(topicModelChange, entry.Display()))
		return nil
	}
}

// listModelsCmd returns a tea.Cmd that lists available models.
func listModelsCmd() tea.Cmd {
	return func() tea.Msg {
		return ModelListResultMsg{Models: listModels()}
	}
}
