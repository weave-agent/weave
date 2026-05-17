package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"weave/sdk"
	"weave/sdk/model"
	"weave/sdk/retry"
)

// ErrorType categorizes an error from the OpenAI-compatible API.
type ErrorType string

const (
	ErrorTypeAuth      ErrorType = "auth"
	ErrorTypeRateLimit ErrorType = "rate_limit"
	ErrorTypeServer    ErrorType = "server"
	ErrorTypeClient    ErrorType = "client"
	ErrorTypeTransport ErrorType = "transport"
	ErrorTypeParse     ErrorType = "parse"
)

// Error represents a structured error from the OpenAI-compatible API.
type Error struct {
	StatusCode int
	Type       ErrorType
	Message    string
	Body       string
}

func (e *Error) Error() string { return fmt.Sprintf("openai-compat: %s: %s", e.Type, e.Message) }

// IsRetriable returns true for rate limit, server, and transport errors.
func (e *Error) IsRetriable() bool {
	switch e.Type {
	case ErrorTypeRateLimit, ErrorTypeServer, ErrorTypeTransport:
		return true
	default:
		return false
	}
}

func classifyStatus(code int) ErrorType {
	switch {
	case code == 401 || code == 403:
		return ErrorTypeAuth
	case code == 429:
		return ErrorTypeRateLimit
	case code >= 500:
		return ErrorTypeServer
	default:
		return ErrorTypeClient
	}
}

// ProviderConfig holds the configuration for an OpenAI-compatible provider.
type ProviderConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	ExtraHeaders  map[string]string
	ExtraBody     map[string]any
	ModifyRequest func(body map[string]any, so *model.StreamOptions)
}

// ChatRequest is the request body sent to the chat completions endpoint.
type ChatRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Stream         bool          `json:"stream"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Tools          []Tool        `json:"tools,omitempty"`
	StreamOptions  *StreamOptions `json:"stream_options,omitempty"`
}

// StreamOptions configures streaming behavior.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
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
	Usage *Usage `json:"usage,omitempty"`
}

// Usage holds token usage information from a streaming response.
type Usage struct {
	InputTokens  int `json:"prompt_tokens"`
	OutputTokens int `json:"completion_tokens"`
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
func Stream(ctx context.Context, client *http.Client, cfg ProviderConfig, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	so := model.NewStreamOptions(opts...)

	mdl := so.Model
	if mdl == "" {
		mdl = cfg.Model
	}

	chatReq := ChatRequest{
		Model:         mdl,
		Messages:      ConvertMessages(req.Messages),
		Stream:        true,
		Tools:         ConvertTools(req.Tools),
		StreamOptions: &StreamOptions{IncludeUsage: true},
	}

	if so.MaxTokens > 0 {
		chatReq.MaxTokens = so.MaxTokens
	}

	if req.SystemPrompt != "" {
		sysMsg := ChatMessage{Role: "system", Content: req.SystemPrompt}
		chatReq.Messages = append([]ChatMessage{sysMsg}, chatReq.Messages...)
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: marshal request: %w", err)
	}

	if len(cfg.ExtraBody) > 0 {
		var bodyMap map[string]any
		if unmarshalErr := json.Unmarshal(reqBody, &bodyMap); unmarshalErr != nil {
			return nil, fmt.Errorf("openai-compat: unmarshal for extra body: %w", unmarshalErr)
		}

		for k, v := range cfg.ExtraBody {
			if _, exists := bodyMap[k]; !exists {
				bodyMap[k] = v
			}
		}

		reqBody, err = json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("openai-compat: marshal extra body: %w", err)
		}
	}

	if cfg.ModifyRequest != nil {
		var bodyMap map[string]any
		if unmarshalErr := json.Unmarshal(reqBody, &bodyMap); unmarshalErr != nil {
			return nil, fmt.Errorf("openai-compat: unmarshal for modify request: %w", unmarshalErr)
		}

		cfg.ModifyRequest(bodyMap, so)

		reqBody, err = json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("openai-compat: marshal modified body: %w", err)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("openai-compat: create request: %w", err)
	}

	httpReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(reqBody)), nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	for k, v := range cfg.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	respBody, err := doStreamRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan sdk.ProviderEvent, 64)

	go func() {
		defer respBody.Close()
		defer close(ch)

		parseSSE(ctx, respBody, ch)
	}()

	return ch, nil
}

// doStreamRequest sends an HTTP request and returns the response body for streaming.
// It closes the body on non-OK statuses and returns an error.
func doStreamRequest(client *http.Client, req *http.Request) (io.ReadCloser, error) {
	resp, err := client.Do(req) //nolint:gosec // G704: URL is constructed from config, not user input
	if err != nil {
		return nil, &Error{Type: ErrorTypeTransport, Message: err.Error()}
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errBody, _ := io.ReadAll(resp.Body)
		bodyStr := string(errBody)

		var errResp ErrorResponse
		if json.Unmarshal(errBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, &Error{
				StatusCode: resp.StatusCode,
				Type:       classifyStatus(resp.StatusCode),
				Message:    errResp.Error.Message,
				Body:       bodyStr,
			}
		}

		return nil, &Error{
			StatusCode: resp.StatusCode,
			Type:       classifyStatus(resp.StatusCode),
			Message:    bodyStr,
			Body:       bodyStr,
		}
	}

	return resp.Body, nil
}

// retryConfig controls retry behavior for stream requests. It is a variable so
// tests can override it with faster settings.
var retryConfig = retry.DefaultConfig()

// doStreamRequestWithRetry wraps doStreamRequest with exponential backoff retry.
func doStreamRequestWithRetry(ctx context.Context, client *http.Client, req *http.Request) (io.ReadCloser, error) {
	var respBody io.ReadCloser

	err := retry.Do(ctx, retryConfig, isRetriableError, func() error {
		if respBody != nil {
			respBody.Close()
			respBody = nil
		}

		if req.GetBody != nil {
			var bodyErr error
			req.Body, bodyErr = req.GetBody()
			if bodyErr != nil {
				return bodyErr
			}
		}

		body, doErr := doStreamRequest(client, req)
		if doErr != nil {
			return doErr
		}

		respBody = body
		return nil
	})

	return respBody, err
}

func isRetriableError(err error) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.IsRetriable()
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

// toolCallAccum accumulates partial tool call data across SSE chunks.
type toolCallAccum struct {
	id         string
	name       string
	argsBuffer string
}

// accumulateToolCall merges a partial ToolCallDelta into the accumulator map.
func accumulateToolCall(toolCalls map[int]*toolCallAccum, tc ToolCallDelta) {
	accumulated, exists := toolCalls[tc.Index]
	if !exists {
		name := ""
		if tc.Function != nil {
			name = tc.Function.Name
		}

		accumulated = &toolCallAccum{id: tc.ID, name: name}
		if accumulated.id == "" {
			accumulated.id = "call_" + strconv.Itoa(tc.Index)
		}

		toolCalls[tc.Index] = accumulated

		if tc.Function != nil {
			accumulated.argsBuffer += tc.Function.Arguments
		}

		return
	}

	if tc.ID != "" {
		accumulated.id = tc.ID
	}

	if tc.Function != nil {
		if tc.Function.Name != "" {
			accumulated.name = tc.Function.Name
		}

		accumulated.argsBuffer += tc.Function.Arguments
	}
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
					Content: &Error{Type: ErrorTypeParse, Message: fmt.Sprintf("parse tool call arguments for %s: %v", tc.name, err)},
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

		data = strings.TrimPrefix(data, " ")
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
				Content: &Error{Type: ErrorTypeParse, Message: fmt.Sprintf("parse chunk: %v", err)},
			})

			return
		}

		// Emit usage event from chunks that carry usage data (typically the final chunk).
		if chunk.Usage != nil && (chunk.Usage.InputTokens > 0 || chunk.Usage.OutputTokens > 0) {
			send(sdk.ProviderEvent{
				Type: sdk.ProviderEventUsage,
				Content: sdk.ProviderUsage{
					InputTokens:  chunk.Usage.InputTokens,
					OutputTokens: chunk.Usage.OutputTokens,
				},
			})
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
				accumulateToolCall(toolCalls, tc)
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
			Content: &Error{Type: ErrorTypeTransport, Message: fmt.Sprintf("read stream: %v", err)},
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
