package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"

	"weave/sdk"
	"weave/sdk/model"
)

const (
	codexTokenURL = "https://auth.openai.com/oauth/token" // #nosec G101 -- OAuth endpoint URL, not a credential.
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
)

// CodexConfig holds per-provider configuration for the Codex provider.
type CodexConfig struct {
	Model   string `json:"model" default:"gpt-5.5" env:"CODEX_MODEL" description:"Model name"`
	BaseURL string `json:"base_url" default:"https://chatgpt.com/backend-api" env:"CODEX_BASE_URL" description:"API base URL"`
}

// AuthConfig holds authentication credentials for the Codex provider (OAuth only).
type AuthConfig struct {
	OAuthToken sdk.OAuthCredential `json:"oauth_token"`
}

type provider struct {
	client     *http.Client
	model      string
	baseURL    string
	tokenURL   string
	oauthToken sdk.OAuthCredential
}

func init() {
	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:          "codex",
		Name:        "Codex",
		ClientID:    codexClientID,
		AuthURL:     "https://auth.openai.com/oauth/authorize",
		TokenURL:    codexTokenURL,
		RedirectURI: "http://localhost:1455/auth/callback",
		ExtraAuthParams: map[string]string{
			"codex_cli_simplified_flow": "true",
			"originator":                "weave",
		},
		Scopes:   []string{"openid", "profile", "email", "offline_access"},
		FlowType: sdk.AuthorizationCode,
	})

	sdk.RegisterProvider("codex", func(cfg sdk.Config, cc CodexConfig, a AuthConfig) (sdk.Provider, error) {
		if a.OAuthToken.AccessToken == "" {
			return nil, errors.New("codex: OAuth token required (use /login codex)")
		}

		return &provider{
			client:     &http.Client{},
			model:      cc.Model,
			baseURL:    cc.BaseURL,
			tokenURL:   codexTokenURL,
			oauthToken: a.OAuthToken,
		}, nil
	})
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	token, err := p.refreshToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}

	so := model.NewStreamOptions(opts...)

	mdl := so.Model
	if mdl == "" {
		mdl = p.model
	}

	body := buildRequestBody(mdl, req, so)

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("codex: marshal request: %w", err)
	}

	endpoint := strings.TrimRight(p.baseURL, "/") + "/codex/responses"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("codex: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	httpReq.Header.Set("originator", "weave")

	accountID, err := extractAccountID(token)
	if err == nil {
		httpReq.Header.Set("chatgpt-account-id", accountID)
	}

	respBody, err := doRequest(p.client, httpReq)
	if err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}

	ch := make(chan sdk.ProviderEvent, 64)

	go func() {
		defer respBody.Close()
		defer close(ch)

		parseResponsesSSE(ctx, respBody, ch)
	}()

	return ch, nil
}

func (p *provider) refreshToken(ctx context.Context) (string, error) {
	if p.oauthToken.AccessToken == "" {
		return "", errors.New("OAuth token required (use /login codex)")
	}

	tokenURL := p.tokenURL
	if tokenURL == "" {
		tokenURL = codexTokenURL
	}

	cred, err := sdk.RefreshOAuthTokenIfNeeded(ctx, "codex", tokenURL, codexClientID, p.oauthToken)
	if err != nil {
		return "", fmt.Errorf("refresh oauth token: %w", err)
	}

	p.oauthToken = cred

	return cred.AccessToken, nil
}

// extractAccountID parses the JWT access token to extract the
// chatgpt_account_id claim needed for the chatgpt-account-id header.
func extractAccountID(accessToken string) (string, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return "", errors.New("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse JWT claims: %w", err)
	}

	if claims.Auth.AccountID == "" {
		return "", errors.New("JWT missing chatgpt_account_id claim")
	}

	return claims.Auth.AccountID, nil
}

var msgIDCounter atomic.Int64

func nextMsgID() string {
	return fmt.Sprintf("msg_%d", msgIDCounter.Add(1))
}

// convertMessages converts SDK messages to the Responses API input format.
// The Responses API uses a flat array of typed items rather than role-based messages.
//
//nolint:goconst // "type" is a standard JSON key, not a magic constant
func convertMessages(msgs []sdk.Message) []any {
	var result []any

	for _, msg := range msgs {
		switch msg.Role {
		case sdk.RoleUser:
			result = append(result, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": fmt.Sprint(msg.Content)},
				},
			})

		case sdk.RoleAssistant:
			if text, ok := msg.Content.(string); ok && text != "" {
				result = append(result, map[string]any{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []map[string]any{
						{"type": "output_text", "text": text},
					},
					"id": nextMsgID(),
				})
			}

			for _, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				result = append(result, map[string]any{
					"type":      "function_call",
					"id":        "fc_" + nextMsgID(),
					"call_id":   tc.ID,
					"name":      tc.Name,
					"arguments": string(argsJSON),
				})
			}

		case sdk.RoleToolResult:
			result = append(result, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  fmt.Sprint(msg.Content),
			})
		}
	}

	return result
}

// convertTools converts SDK tool definitions to the Responses API flat tool format.
func convertTools(tools []sdk.ToolDef) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		params := make(map[string]any)
		if p, ok := t.Parameters.(map[string]any); ok {
			params = p
		}

		result[i] = map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		}
	}

	return result
}

// buildRequestBody constructs the Responses API request body.
func buildRequestBody(modelName string, req sdk.ProviderRequest, so *model.StreamOptions) map[string]any {
	input := convertMessages(req.Messages)

	body := map[string]any{
		"model":   modelName,
		"store":   false,
		"stream":  true,
		"input":   input,
		"text":    map[string]any{"verbosity": "low"},
		"include": []string{"reasoning.encrypted_content"},
	}

	if req.SystemPrompt != "" {
		body["instructions"] = req.SystemPrompt
	}

	if so.ThinkingLevel != model.ThinkingOff {
		effortMap := map[model.ThinkingLevel]string{
			model.ThinkingMinimal: "low",
			model.ThinkingLow:     "low",
			model.ThinkingMedium:  "medium",
			model.ThinkingHigh:    "high",
			model.ThinkingXHigh:   "high",
		}

		effort := "medium"
		if e, ok := effortMap[so.ThinkingLevel]; ok {
			effort = e
		}

		body["reasoning"] = map[string]any{
			"effort":  effort,
			"summary": "auto",
		}
	}

	if tools := convertTools(req.Tools); tools != nil {
		body["tools"] = tools
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = true
	}

	if so.MaxTokens > 0 {
		body["max_output_tokens"] = so.MaxTokens
	}

	return body
}

// doRequest sends an HTTP request and returns the response body for streaming.
func doRequest(client *http.Client, req *http.Request) (io.ReadCloser, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errBody, _ := io.ReadAll(resp.Body)
		bodyStr := string(errBody)

		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(errBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}

		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, bodyStr)
	}

	return resp.Body, nil
}

// toolCallAccum accumulates partial tool call data across SSE deltas.
type toolCallAccum struct {
	id         string
	callID     string
	name       string
	argsBuffer string
}

func emitToolCall(accum *toolCallAccum, send func(sdk.ProviderEvent) bool) {
	var args map[string]any
	if accum.argsBuffer != "" {
		if err := json.Unmarshal([]byte(accum.argsBuffer), &args); err != nil {
			send(sdk.ProviderEvent{
				Type:    sdk.ProviderEventError,
				Content: fmt.Sprintf("codex: parse tool call arguments for %s: %v", accum.name, err),
			})

			return
		}
	}

	if args == nil {
		args = make(map[string]any)
	}

	toolCallID := accum.callID
	if toolCallID == "" {
		toolCallID = accum.id
	}

	send(sdk.ProviderEvent{
		Type: sdk.ProviderEventToolCall,
		Content: sdk.ToolCall{
			ID:        toolCallID,
			Name:      accum.name,
			Arguments: args,
		},
	})
}

// parseResponsesSSE reads a Responses API SSE stream and emits ProviderEvents.
// The Responses API uses typed events (event: X\ndata: {...}\n\n) instead of
// the Chat Completions data-only format.
func parseResponsesSSE(ctx context.Context, reader io.Reader, ch chan<- sdk.ProviderEvent) {
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
	currentEventType := ""

	for scanner.Scan() {
		line := scanner.Text()

		if after, ok := strings.CutPrefix(line, "event:"); ok {
			currentEventType = strings.TrimSpace(after)
			continue
		}

		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}

		data = strings.TrimPrefix(data, " ")
		if data == "" {
			continue
		}

		if handleSSEEvent(currentEventType, data, toolCalls, send) {
			return
		}

		currentEventType = ""
	}

	emitRemainingToolCalls(toolCalls, send)

	if err := scanner.Err(); err != nil {
		send(sdk.ProviderEvent{
			Type:    sdk.ProviderEventError,
			Content: fmt.Errorf("codex: read stream: %w", err),
		})
	}
}

// handleSSEEvent processes a single SSE event. Returns true if the stream should end.
func handleSSEEvent(eventType, data string, toolCalls map[int]*toolCallAccum, send func(sdk.ProviderEvent) bool) bool {
	switch eventType {
	case "response.output_text.delta":
		handleDelta(data, sdk.ProviderEventTextDelta, send)

	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		handleDelta(data, sdk.ProviderEventThinking, send)

	case "response.output_item.added":
		handleItemAdded(data, toolCalls)

	case "response.function_call_arguments.delta":
		handleArgsDelta(data, toolCalls)

	case "response.output_item.done":
		handleItemDone(data, toolCalls, send)

	case "response.completed":
		return true

	case "response.failed":
		handleFailed(data, send)
		return true
	}

	return false
}

func handleDelta(data, evtType string, send func(sdk.ProviderEvent) bool) {
	var evt struct {
		Delta string `json:"delta"`
	}
	if json.Unmarshal([]byte(data), &evt) == nil && evt.Delta != "" {
		send(sdk.ProviderEvent{Type: evtType, Content: evt.Delta})
	}
}

func handleItemAdded(data string, toolCalls map[int]*toolCallAccum) {
	var evt struct {
		OutputIndex int `json:"output_index"`
		Item        struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
		} `json:"item"`
	}
	if json.Unmarshal([]byte(data), &evt) == nil && evt.Item.Type == "function_call" {
		toolCalls[evt.OutputIndex] = &toolCallAccum{
			id:     evt.Item.ID,
			callID: evt.Item.CallID,
			name:   evt.Item.Name,
		}
	}
}

func handleArgsDelta(data string, toolCalls map[int]*toolCallAccum) {
	var evt struct {
		OutputIndex int    `json:"output_index"`
		Delta       string `json:"delta"`
	}
	if json.Unmarshal([]byte(data), &evt) == nil {
		accum, exists := toolCalls[evt.OutputIndex]
		if !exists {
			accum = &toolCallAccum{}
			toolCalls[evt.OutputIndex] = accum
		}

		accum.argsBuffer += evt.Delta
	}
}

func handleItemDone(data string, toolCalls map[int]*toolCallAccum, send func(sdk.ProviderEvent) bool) {
	var evt struct {
		OutputIndex int `json:"output_index"`
		Item        struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if json.Unmarshal([]byte(data), &evt) == nil && evt.Item.Type == "function_call" {
		if accum, ok := toolCalls[evt.OutputIndex]; ok {
			emitToolCall(accum, send)
			delete(toolCalls, evt.OutputIndex)
		}
	}
}

func handleFailed(data string, send func(sdk.ProviderEvent) bool) {
	var evt struct {
		Response struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}

	msg := "response failed"
	if json.Unmarshal([]byte(data), &evt) == nil && evt.Response.Error.Message != "" {
		msg = evt.Response.Error.Message
	}

	send(sdk.ProviderEvent{
		Type:    sdk.ProviderEventError,
		Content: fmt.Errorf("codex: %s", msg),
	})
}

func emitRemainingToolCalls(toolCalls map[int]*toolCallAccum, send func(sdk.ProviderEvent) bool) {
	keys := make([]int, 0, len(toolCalls))
	for k := range toolCalls {
		keys = append(keys, k)
	}

	sort.Ints(keys)

	for _, k := range keys {
		emitToolCall(toolCalls[k], send)
	}
}
