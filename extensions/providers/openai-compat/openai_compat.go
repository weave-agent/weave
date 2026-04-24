package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"weave/sdk"
)

const defaultModel = "gpt-4o"

// ProviderConfig holds the configuration for an OpenAI-compatible provider.
type ProviderConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// ChatRequest is the request body sent to the chat completions endpoint.
type ChatRequest struct {
	Model           string        `json:"model"`
	Messages        []ChatMessage `json:"messages"`
	Stream          bool          `json:"stream"`
	MaxTokens       int64         `json:"max_tokens,omitempty"`
	Tools           []Tool        `json:"tools,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

// ChatMessage represents a single message in the OpenAI chat format.
type ChatMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []StreamTool `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

// Tool represents an OpenAI function tool definition.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function describes a tool function.
type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// StreamChunk is a streaming response chunk from the chat completions endpoint.
type StreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int        `json:"index"`
		Delta        ChunkDelta `json:"delta"`
		FinishReason *string    `json:"finish_reason"`
	} `json:"choices"`
}

// ChunkDelta represents the delta content in a streaming chunk.
type ChunkDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ToolCallDelta represents a partial tool call in a streaming chunk.
type ToolCallDelta struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function *FunctionCallDelta `json:"function,omitempty"`
}

// FunctionCallDelta represents partial function call data.
type FunctionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// StreamTool is a completed tool call in a non-streaming response.
type StreamTool struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ErrorResponse represents an error from the API.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Stream sends a request to an OpenAI-compatible API and returns a channel of ProviderEvents.
func Stream(ctx context.Context, client *http.Client, cfg ProviderConfig, req sdk.ProviderRequest, opts ...sdk.StreamOption) (<-chan sdk.ProviderEvent, error) {
	so := sdk.NewStreamOptions(opts...)

	model := so.Model
	if model == "" {
		model = cfg.Model
	}
	if model == "" {
		model = defaultModel
	}

	chatReq := ChatRequest{
		Model:    model,
		Messages: ConvertMessages(req.Messages),
		Stream:   true,
		Tools:    ConvertTools(req.Tools),
	}

	if so.MaxTokens > 0 {
		chatReq.MaxTokens = so.MaxTokens
	}

	effortMap := map[sdk.ThinkingLevel]string{
		sdk.ThinkingMinimal: "low",
		sdk.ThinkingLow:     "low",
		sdk.ThinkingMedium:  "medium",
		sdk.ThinkingHigh:    "high",
		sdk.ThinkingXHigh:   "high",
	}

	if so.ThinkingLevel != sdk.ThinkingOff {
		if effort, ok := effortMap[so.ThinkingLevel]; ok {
			chatReq.ReasoningEffort = effort
		}
	}

	if req.SystemPrompt != "" {
		sysMsg := ChatMessage{Role: "system", Content: req.SystemPrompt}
		chatReq.Messages = append([]ChatMessage{sysMsg}, chatReq.Messages...)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai-compat: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(errBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai-compat: API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai-compat: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan sdk.ProviderEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)
		parseSSE(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// toolCallAccum accumulates partial tool call data across SSE chunks.
type toolCallAccum struct {
	id         string
	name       string
	argsBuffer string
}

// emitToolCalls sorts and emits accumulated tool calls as ProviderEvents.
func emitToolCalls(toolCalls map[int]*toolCallAccum, send func(sdk.ProviderEvent) bool) {
	if len(toolCalls) == 0 {
		return
	}

	keys := make([]int, 0, len(toolCalls))

	for k := range toolCalls {
		keys = append(keys, k)
	}

	sort.Ints(keys)

	for _, k := range keys {
		tc := toolCalls[k]

		var args map[string]any

		if tc.argsBuffer != "" {
			if err := json.Unmarshal([]byte(tc.argsBuffer), &args); err != nil {
				send(sdk.ProviderEvent{
					Type:    sdk.ProviderEventError,
					Content: fmt.Sprintf("openai-compat: parse tool call arguments for %s: %v", tc.name, err),
				})

				continue
			}
		}

		if args == nil {
			args = make(map[string]any)
		}

		if !send(sdk.ProviderEvent{
			Type: sdk.ProviderEventToolCall,
			Content: sdk.ToolCall{
				ID:        tc.id,
				Name:      tc.name,
				Arguments: args,
			},
		}) {
			return
		}
	}
}

// parseSSE reads an SSE stream and emits ProviderEvents.
// It respects context cancellation to prevent goroutine leaks.
func parseSSE(ctx context.Context, reader io.Reader, ch chan<- sdk.ProviderEvent) {
	send := func(evt sdk.ProviderEvent) bool {
		select {
		case ch <- evt:
			return true
		case <-ctx.Done():
			return false
		}
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	toolCalls := make(map[int]*toolCallAccum)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || line[0] == ':' {
			continue
		}

		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}

		data = strings.TrimLeft(data, " ")
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			send(sdk.ProviderEvent{
				Type:    sdk.ProviderEventError,
				Content: fmt.Sprintf("openai-compat: parse chunk: %v", err),
			})
			return
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if !send(sdk.ProviderEvent{
					Type:    sdk.ProviderEventTextDelta,
					Content: choice.Delta.Content,
				}) {
					return
				}
			}

			reasoning := choice.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = choice.Delta.Reasoning
			}
			if reasoning != "" {
				if !send(sdk.ProviderEvent{
					Type:    sdk.ProviderEventThinking,
					Content: reasoning,
				}) {
					return
				}
			}

			for _, tc := range choice.Delta.ToolCalls {
				accumulated, exists := toolCalls[tc.Index]
				if !exists {
					var name string
					if tc.Function != nil {
						name = tc.Function.Name
					}
					accumulated = &toolCallAccum{id: tc.ID, name: name}
					if accumulated.id == "" {
						accumulated.id = "call_" + strconv.Itoa(tc.Index)
					}
					toolCalls[tc.Index] = accumulated
				} else {
					if tc.ID != "" {
						accumulated.id = tc.ID
					}
					if tc.Function != nil && tc.Function.Name != "" {
						accumulated.name = tc.Function.Name
					}
				}

				if tc.Function != nil {
					accumulated.argsBuffer += tc.Function.Arguments
				}
			}

			if choice.FinishReason != nil && *choice.FinishReason == "tool_calls" {
				emitToolCalls(toolCalls, send)
				toolCalls = make(map[int]*toolCallAccum)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		send(sdk.ProviderEvent{
			Type:    sdk.ProviderEventError,
			Content: fmt.Sprintf("openai-compat: read stream: %v", err),
		})
		return
	}

	// Emit any tool calls accumulated but never flushed by a finish_reason
	// (e.g. stream ended without one, or the API omitted it).
	emitToolCalls(toolCalls, send)
}

// ConvertMessages converts SDK messages to OpenAI chat format.
func ConvertMessages(msgs []sdk.Message) []ChatMessage {
	var result []ChatMessage

	for _, msg := range msgs {
		switch msg.Role {
		case sdk.RoleUser:
			result = append(result, ChatMessage{
				Role:    "user",
				Content: fmt.Sprint(msg.Content),
			})
		case sdk.RoleAssistant:
			cm := ChatMessage{Role: "assistant"}
			if text, ok := msg.Content.(string); ok && text != "" {
				cm.Content = text
			}
			for _, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				var st StreamTool
				st.ID = tc.ID
				st.Type = "function"
				st.Function.Name = tc.Name
				st.Function.Arguments = string(argsJSON)
				cm.ToolCalls = append(cm.ToolCalls, st)
			}
			result = append(result, cm)
		case sdk.RoleToolResult:
			cm := ChatMessage{
				Role:       "tool",
				Content:    fmt.Sprint(msg.Content),
				ToolCallID: msg.ToolCallID,
			}
			result = append(result, cm)
		}
	}

	return result
}

// ConvertTools converts SDK tool definitions to OpenAI format.
func ConvertTools(tools []sdk.ToolDef) []Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]Tool, len(tools))
	for i, t := range tools {
		params := make(map[string]any)
		if p, ok := t.Parameters.(map[string]any); ok {
			params = p
		}
		result[i] = Tool{
			Type: "function",
			Function: Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		}
	}
	return result
}
