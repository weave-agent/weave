package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	tea "charm.land/bubbletea/v2"
)

// Bus event topics (matching agent-loop topics).
const (
	topicPrompt    = "agent.prompt"
	topicSteer     = "agent.steer"
	topicFollowup  = "agent.followup"
	topicInterrupt = "agent.interrupt"

	topicTurnStart  = "agent.turn_start"
	topicTurnEnd    = "agent.turn_end"
	topicMsgStart   = "agent.message_start"
	topicMsgUpdate  = "agent.message_update"
	topicMsgEnd     = "agent.message_end"
	topicToolResult = "agent.tool_result"
	topicUsage      = "agent.usage"
	topicEnd        = "agent.end"

	topicSessionList       = "session.list"
	topicSessionResume     = "session.resume"
	topicModelChange       = "model.change"
	topicModelChangeFailed = "model.change_failed"
	topicThinkingChange    = "thinking.change"

	topicCompacted = "agent.compacted"

	topicExtOutdated = "extension.outdated"

	topicAuthLoginSuccess = "auth.login.success"
	topicAuthLogout       = "auth.logout"

	keyProvider = "provider"
	keyModel    = "model"
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
	Content   string
	TokenRate float64
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
	Messages  []sdk.Message
}

// ModelListResultMsg carries the result of listing available models.
type ModelListResultMsg struct {
	Models []ModelEntry
}

// ModelChangedMsg is sent when the user selects or cycles to a new model.
type ModelChangedMsg struct {
	Entry ModelEntry
}

// ModelChangeFailedMsg is sent when the loop fails to switch providers.
type ModelChangeFailedMsg struct {
	Provider string
	Error    string
}

// ThinkingLevelSetMsg is sent when the user sets the thinking level via /thinking command.
type ThinkingLevelSetMsg struct {
	Level sdkmodel.ThinkingLevel
}

// OutdatedNotificationMsg is sent when outdated extensions are detected at startup.
type OutdatedNotificationMsg struct {
	Extensions []sdk.OutdatedInfo
}

// CompactedMsg is sent when the agent compacts the conversation context.
type CompactedMsg struct {
	Summarized   int
	TokensBefore int
	TokensAfter  int
	Error        string
}

// TokenUsageMsg is sent when the provider reports token usage for a turn.
type TokenUsageMsg struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
}

// AgentStateChangeMsg is sent when the agent activity state changes.
// The UI updates accent colors and editor pulse animation based on this.
type AgentStateChangeMsg struct {
	State palette.State
}

// agentStateTracker tracks agent activity state from bus events.
// It lives in the Bridge goroutine and sends state changes to the program.
type agentStateTracker struct {
	state     palette.State
	toolCount int // pending tool calls awaiting results
}

func newAgentStateTracker() *agentStateTracker {
	return &agentStateTracker{state: palette.StateIdle}
}

// update computes the new state based on an incoming event message.
// Returns the new state and whether it changed.
func (t *agentStateTracker) update(msg tea.Msg) (palette.State, bool) {
	prev := t.state

	switch msg := msg.(type) {
	case TurnStartMsg:
		t.state = palette.StateStreaming
		t.toolCount = 0
	case MessageStartMsg:
		t.state = palette.StateStreaming
	case ToolResultMsg:
		if t.toolCount > 0 {
			t.toolCount--
		}

		if t.toolCount > 0 {
			t.state = palette.StateToolRunning
		} else if t.state == palette.StateToolRunning {
			t.state = palette.StateStreaming
		}
	case MessageEndMsg:
		if len(msg.ToolCalls) > 0 {
			t.toolCount += len(msg.ToolCalls)
			t.state = palette.StateToolRunning
		}
	case TurnEndMsg:
		t.state = palette.StateIdle
		t.toolCount = 0
	case AgentEndMsg:
		if errStr, ok := msg.Payload.(string); ok && errStr != "" {
			t.state = palette.StateError
		} else {
			t.state = palette.StateIdle
		}

		t.toolCount = 0
	}

	return t.state, t.state != prev
}

// ProviderListResultMsg carries the result of listing providers with key status.
type ProviderListResultMsg struct {
	Providers []ProviderEntry
}

// LoginListResultMsg carries the result of listing providers available for login.
type LoginListResultMsg struct {
	Providers []LoginProviderEntry
}

// LogoutListResultMsg carries the result of listing providers with configured auth.
type LogoutListResultMsg struct {
	Providers []LogoutProviderEntry
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
		if p, ok := evt.Payload.(sdk.SessionResumePayload); ok {
			return SessionResumedMsg{SessionID: p.SessionID, Messages: p.Messages}
		}

		return SessionResumedMsg{}
	case topicModelChangeFailed:
		return translateModelChangeFailed(evt.Payload)
	case topicExtOutdated:
		return translateExtOutdated(evt.Payload)
	case topicCompacted:
		return translateCompacted(evt.Payload)
	case topicUsage:
		return translateUsage(evt.Payload)
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

func translateModelChangeFailed(payload any) ModelChangeFailedMsg {
	m, ok := payload.(map[string]any)
	if !ok {
		return ModelChangeFailedMsg{}
	}

	provider, _ := m[keyProvider].(string)
	errStr, _ := m["error"].(string)

	return ModelChangeFailedMsg{Provider: provider, Error: errStr}
}

func translateExtOutdated(payload any) OutdatedNotificationMsg {
	evt, ok := payload.(sdk.OutdatedEvent)
	if !ok {
		return OutdatedNotificationMsg{}
	}

	return OutdatedNotificationMsg{Extensions: evt.Extensions}
}

func translateCompacted(payload any) CompactedMsg {
	m, ok := payload.(map[string]any)
	if !ok {
		return CompactedMsg{}
	}

	if errStr, ok := m["error"].(string); ok {
		return CompactedMsg{Error: errStr}
	}

	summarized, _ := m["summarized"].(int)
	tokensBefore, _ := m["tokens_before"].(int)
	tokensAfter, _ := m["tokens_after"].(int)

	return CompactedMsg{
		Summarized:   summarized,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
	}
}

func translateUsage(payload any) TokenUsageMsg {
	m, ok := payload.(map[string]any)
	if !ok {
		return TokenUsageMsg{}
	}

	inputTokens, _ := m["input_tokens"].(int)
	outputTokens, _ := m["output_tokens"].(int)
	cacheCreationTokens, _ := m["cache_creation_tokens"].(int)
	cacheReadTokens, _ := m["cache_read_tokens"].(int)

	return TokenUsageMsg{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheCreationTokens: cacheCreationTokens,
		CacheReadTokens:     cacheReadTokens,
	}
}

// Bridge reads bus events and sends them as tea.Msg to the program.
// When multiple MessageUpdateMsg deltas arrive in rapid succession, it batches
// them into a single concatenated message to reduce UI update pressure.
// Blocks until the event channel is closed.
func Bridge(sender Sender, events <-chan sdk.Event) {
	var (
		streamStart  time.Time
		streamRunes  int
		stateTracker = newAgentStateTracker()
	)

	for evt := range events {
		msg := translateEvent(evt)
		if msg == nil {
			continue
		}

		// Track agent state changes
		if newState, changed := stateTracker.update(msg); changed {
			sender.Send(AgentStateChangeMsg{State: newState})
		}

		// Reset rate tracking at message boundaries
		switch msg.(type) {
		case MessageStartMsg, MessageEndMsg:
			streamStart = time.Time{}
			streamRunes = 0
		}

		// Batch consecutive MessageUpdateMsg deltas
		if mu, ok := msg.(MessageUpdateMsg); ok { //nolint:nestif // batching requires nested select/drain
			// Track rate
			if streamStart.IsZero() {
				streamStart = time.Now()
			}

			streamRunes += utf8.RuneCountInString(mu.Content)

			var batch strings.Builder

			batch.WriteString(mu.Content)

			// Drain any queued deltas
			draining := true
			for draining {
				select {
				case next, ok := <-events:
					if !ok {
						// Channel closed while batching — flush and exit
						if batch.Len() > 0 {
							sender.Send(MessageUpdateMsg{
								Content:   batch.String(),
								TokenRate: calcTokenRate(streamStart, streamRunes),
							})
						}

						sender.Send(ShutdownMsg{})

						return
					}

					nextMsg := translateEvent(next)
					if nextMu, ok := nextMsg.(MessageUpdateMsg); ok {
						streamRunes += utf8.RuneCountInString(nextMu.Content)
						batch.WriteString(nextMu.Content)
					} else {
						// Non-delta message — flush the batch, then handle this message
						if batch.Len() > 0 {
							sender.Send(MessageUpdateMsg{
								Content:   batch.String(),
								TokenRate: calcTokenRate(streamStart, streamRunes),
							})
							batch.Reset()
						}

						// Track state for non-delta messages found during drain
						if newState, changed := stateTracker.update(nextMsg); changed {
							sender.Send(AgentStateChangeMsg{State: newState})
						}

						// Reset rate at message boundaries found during drain
						switch nextMsg.(type) {
						case MessageStartMsg, MessageEndMsg:
							streamStart = time.Time{}
							streamRunes = 0
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
				sender.Send(MessageUpdateMsg{
					Content:   batch.String(),
					TokenRate: calcTokenRate(streamStart, streamRunes),
				})
			}

			continue
		}

		sender.Send(msg)
	}

	sender.Send(ShutdownMsg{})
}

// calcTokenRate estimates token rate from accumulated rune count and elapsed time.
// Uses the standard heuristic of 1 token ≈ 4 characters.
func calcTokenRate(start time.Time, totalRunes int) float64 {
	if start.IsZero() {
		return 0
	}

	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0
	}

	return float64(totalRunes) / 4.0 / elapsed
}

// PublishPrompt returns a tea.Cmd that publishes an agent.prompt event.
func PublishPrompt(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicPrompt, text))
		}

		return nil
	}
}

// PublishFollowup returns a tea.Cmd that publishes an agent.followup event.
func PublishFollowup(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicFollowup, text))
		}

		return nil
	}
}

// PublishSteer returns a tea.Cmd that publishes an agent.steer event.
func PublishSteer(bus sdk.Bus, text string) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicSteer, text))
		}

		return nil
	}
}

// PublishInterrupt returns a tea.Cmd that publishes an agent.interrupt event.
func PublishInterrupt(bus sdk.Bus) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicInterrupt, "user interrupt"))
		}

		return nil
	}
}

// PublishSessionResume returns a tea.Cmd that publishes a session.resume event.
func PublishSessionResume(bus sdk.Bus, payload sdk.SessionResumePayload) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicSessionResume, payload))
		}

		return nil
	}
}

// PublishModelChange returns a tea.Cmd that publishes a model.change event.
func PublishModelChange(bus sdk.Bus, entry ModelEntry) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicModelChange, map[string]string{
				keyProvider: entry.Provider,
				keyModel:    entry.Model,
			}))
		}

		return nil
	}
}

// PublishThinkingChange returns a tea.Cmd that publishes a thinking.change event.
func PublishThinkingChange(bus sdk.Bus, level sdkmodel.ThinkingLevel) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicThinkingChange, map[string]string{
				"level": string(level),
			}))
		}

		return nil
	}
}

// PublishAuthLoginSuccess returns a tea.Cmd that publishes an auth.login.success event.
func PublishAuthLoginSuccess(bus sdk.Bus, provider string) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicAuthLoginSuccess, map[string]string{
				keyProvider: provider,
			}))
		}

		return nil
	}
}

// PublishAuthLogout returns a tea.Cmd that publishes an auth.logout event.
func PublishAuthLogout(bus sdk.Bus, provider string) tea.Cmd {
	return func() tea.Msg {
		if bus != nil {
			bus.Publish(sdk.NewEvent(topicAuthLogout, map[string]string{
				keyProvider: provider,
			}))
		}

		return nil
	}
}

// listModelsCmd returns a tea.Cmd that lists available models.
func listModelsCmd() tea.Cmd {
	return func() tea.Msg {
		return ModelListResultMsg{Models: listModels()}
	}
}

// listProvidersCmd returns a tea.Cmd that lists providers with key status.
func listProvidersCmd() tea.Cmd {
	return func() tea.Msg {
		return ProviderListResultMsg{Providers: listProviders()}
	}
}

// loginCmd returns a tea.Cmd that lists providers available for login.
func loginCmd() tea.Cmd {
	return func() tea.Msg {
		return LoginListResultMsg{Providers: buildLoginProviders()}
	}
}

// logoutCmd returns a tea.Cmd that lists providers with configured auth.
func logoutCmd() tea.Cmd {
	return func() tea.Msg {
		return LogoutListResultMsg{Providers: buildLogoutProviders()}
	}
}
